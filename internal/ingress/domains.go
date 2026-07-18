package ingress

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	StatusPending           = "pending"
	StatusCheckingDNS       = "checking_dns"
	StatusActive            = "active"
	StatusDNSError          = "dns_error"
	StatusCertificateError  = "certificate_error"
	StatusTargetUnavailable = "target_unavailable"
	StatusDisabled          = "disabled"
)

var (
	ErrNotFound      = errors.New("domain not found")
	ErrInvalidDomain = errors.New("invalid domain")
	hostnamePattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
	servicePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
)

type Domain struct {
	ID                int64  `json:"id"`
	ApplicationID     int64  `json:"application_id"`
	Hostname          string `json:"hostname"`
	ServiceName       string `json:"service_name"`
	TargetPort        int    `json:"target_port"`
	Primary           bool   `json:"primary_domain"`
	HTTPSEnabled      bool   `json:"https_enabled"`
	RedirectHTTPS     bool   `json:"redirect_https"`
	Enabled           bool   `json:"enabled"`
	Status            string `json:"status"`
	CertificateStatus string `json:"certificate_status"`
	LastError         string `json:"last_error,omitempty"`
	LastCheckedAt     string `json:"last_checked_at,omitempty"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type CreateInput struct {
	Hostname      string `json:"hostname"`
	ServiceName   string `json:"service_name"`
	TargetPort    int    `json:"target_port"`
	Primary       bool   `json:"primary_domain"`
	HTTPSEnabled  *bool  `json:"https_enabled"`
	RedirectHTTPS *bool  `json:"redirect_https"`
}

type UpdateInput struct {
	ServiceName   *string `json:"service_name"`
	TargetPort    *int    `json:"target_port"`
	Primary       *bool   `json:"primary_domain"`
	HTTPSEnabled  *bool   `json:"https_enabled"`
	RedirectHTTPS *bool   `json:"redirect_https"`
	Enabled       *bool   `json:"enabled"`
}

func ValidateHostname(value string) (string, error) {
	hostname := strings.ToLower(strings.TrimSpace(value))
	if len(hostname) < 3 || len(hostname) > 253 || strings.HasSuffix(hostname, ".local") || !hostnamePattern.MatchString(hostname) {
		return "", ErrInvalidDomain
	}
	if net.ParseIP(hostname) != nil || hostname == "localhost" || strings.ContainsAny(hostname, "/:@*?[]") {
		return "", ErrInvalidDomain
	}
	return hostname, nil
}

func ValidateServiceName(value string) bool {
	return servicePattern.MatchString(strings.TrimSpace(value))
}

type Store interface {
	Create(context.Context, int64, CreateInput) (Domain, error)
	Get(context.Context, int64, int64) (Domain, error)
	List(context.Context, int64) ([]Domain, error)
	ListEnabled(context.Context) ([]Domain, error)
	Update(context.Context, int64, int64, UpdateInput) (Domain, error)
	Delete(context.Context, int64, int64) error
	MarkCheck(context.Context, int64, string, string, string) error
}

type SQLStore struct{ db *sql.DB }

func NewSQLStore(db *sql.DB) *SQLStore { return &SQLStore{db: db} }

const domainColumns = `id,application_id,hostname,service_name,target_port,primary_domain,https_enabled,redirect_https,enabled,status,certificate_status,last_error,coalesce(last_checked_at,''),created_at,updated_at`

func (s *SQLStore) Create(ctx context.Context, applicationID int64, input CreateInput) (Domain, error) {
	hostname, err := ValidateHostname(input.Hostname)
	if err != nil || input.TargetPort < 1 || input.TargetPort > 65535 || !ValidateServiceName(input.ServiceName) {
		return Domain{}, ErrInvalidDomain
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	httpsEnabled, redirectHTTPS := true, true
	if input.HTTPSEnabled != nil {
		httpsEnabled = *input.HTTPSEnabled
	}
	if input.RedirectHTTPS != nil {
		redirectHTTPS = *input.RedirectHTTPS
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Domain{}, fmt.Errorf("begin domain: %w", err)
	}
	defer tx.Rollback()
	if input.Primary {
		if _, err := tx.ExecContext(ctx, `UPDATE application_domains SET primary_domain=0 WHERE application_id=?`, applicationID); err != nil {
			return Domain{}, err
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO application_domains(application_id,hostname,service_name,target_port,primary_domain,https_enabled,redirect_https,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, applicationID, hostname, strings.TrimSpace(input.ServiceName), input.TargetPort, boolInt(input.Primary), boolInt(httpsEnabled), boolInt(redirectHTTPS), now, now)
	if err != nil {
		return Domain{}, fmt.Errorf("create domain: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Domain{}, err
	}
	if err := tx.Commit(); err != nil {
		return Domain{}, err
	}
	return s.Get(ctx, applicationID, id)
}

func (s *SQLStore) Get(ctx context.Context, applicationID, id int64) (Domain, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+domainColumns+` FROM application_domains WHERE application_id=? AND id=?`, applicationID, id)
	item, err := scanDomain(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Domain{}, ErrNotFound
	}
	if err != nil {
		return Domain{}, fmt.Errorf("get domain: %w", err)
	}
	return item, nil
}

func (s *SQLStore) List(ctx context.Context, applicationID int64) ([]Domain, error) {
	return s.list(ctx, `WHERE application_id=?`, applicationID)
}

func (s *SQLStore) ListEnabled(ctx context.Context) ([]Domain, error) {
	return s.list(ctx, `WHERE enabled=1`, nil)
}

func (s *SQLStore) list(ctx context.Context, clause string, argument any) ([]Domain, error) {
	query := `SELECT ` + domainColumns + ` FROM application_domains ` + clause + ` ORDER BY primary_domain DESC, id ASC`
	var rows *sql.Rows
	var err error
	if argument == nil {
		rows, err = s.db.QueryContext(ctx, query)
	} else {
		rows, err = s.db.QueryContext(ctx, query, argument)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Domain{}
	for rows.Next() {
		item, scanErr := scanDomain(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *SQLStore) Update(ctx context.Context, applicationID, id int64, input UpdateInput) (Domain, error) {
	current, err := s.Get(ctx, applicationID, id)
	if err != nil {
		return Domain{}, err
	}
	if input.ServiceName != nil {
		current.ServiceName = strings.TrimSpace(*input.ServiceName)
	}
	if input.TargetPort != nil {
		current.TargetPort = *input.TargetPort
	}
	if input.Primary != nil {
		current.Primary = *input.Primary
	}
	if input.HTTPSEnabled != nil {
		current.HTTPSEnabled = *input.HTTPSEnabled
	}
	if input.RedirectHTTPS != nil {
		current.RedirectHTTPS = *input.RedirectHTTPS
	}
	if input.Enabled != nil {
		current.Enabled = *input.Enabled
	}
	if !ValidateServiceName(current.ServiceName) || current.TargetPort < 1 || current.TargetPort > 65535 {
		return Domain{}, ErrInvalidDomain
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Domain{}, err
	}
	defer tx.Rollback()
	if current.Primary {
		if _, err := tx.ExecContext(ctx, `UPDATE application_domains SET primary_domain=0 WHERE application_id=?`, applicationID); err != nil {
			return Domain{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE application_domains SET service_name=?,target_port=?,primary_domain=?,https_enabled=?,redirect_https=?,enabled=?,updated_at=? WHERE application_id=? AND id=?`, current.ServiceName, current.TargetPort, boolInt(current.Primary), boolInt(current.HTTPSEnabled), boolInt(current.RedirectHTTPS), boolInt(current.Enabled), now, applicationID, id)
	if err != nil {
		return Domain{}, err
	}
	if err := tx.Commit(); err != nil {
		return Domain{}, err
	}
	return s.Get(ctx, applicationID, id)
}

func (s *SQLStore) Delete(ctx context.Context, applicationID, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM application_domains WHERE application_id=? AND id=?`, applicationID, id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLStore) MarkCheck(ctx context.Context, id int64, status, certificateStatus, lastError string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE application_domains SET status=?,certificate_status=?,last_error=?,last_checked_at=?,updated_at=? WHERE id=?`, status, certificateStatus, lastError, time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

type scanFunc func(...any) error

func scanDomain(scan scanFunc) (Domain, error) {
	var item Domain
	var primary, httpsEnabled, redirectHTTPS, enabled int
	if err := scan(&item.ID, &item.ApplicationID, &item.Hostname, &item.ServiceName, &item.TargetPort, &primary, &httpsEnabled, &redirectHTTPS, &enabled, &item.Status, &item.CertificateStatus, &item.LastError, &item.LastCheckedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return Domain{}, err
	}
	item.Primary, item.HTTPSEnabled, item.RedirectHTTPS, item.Enabled = primary != 0, httpsEnabled != 0, redirectHTTPS != 0, enabled != 0
	return item, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type TargetResolver interface {
	Resolve(context.Context, Domain) (string, error)
}

type StaticResolver map[string]string

func (r StaticResolver) Resolve(_ context.Context, domain Domain) (string, error) {
	target := r[domain.Hostname]
	if target == "" {
		return "", errors.New("target unavailable")
	}
	return target, nil
}

type route struct {
	domain Domain
	proxy  *httputil.ReverseProxy
}

type Proxy struct {
	routes    atomic.Value
	transport http.RoundTripper
}

func NewProxy(transports ...http.RoundTripper) *Proxy {
	p := &Proxy{}
	if len(transports) > 0 {
		p.transport = transports[0]
	}
	p.routes.Store(map[string]route{})
	return p
}

func (p *Proxy) SetRoutes(ctx context.Context, domains []Domain, resolver TargetResolver) {
	routes := make(map[string]route, len(domains))
	for _, domain := range domains {
		if !domain.Enabled {
			continue
		}
		target, err := resolver.Resolve(ctx, domain)
		if err != nil {
			continue
		}
		targetURL, err := url.Parse(target)
		if err != nil || (targetURL.Scheme != "http" && targetURL.Scheme != "https") || targetURL.Host == "" {
			continue
		}
		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		if p.transport != nil {
			proxy.Transport = p.transport
		}
		originalDirector := proxy.Director
		proxy.Director = func(request *http.Request) {
			originalHost := request.Host
			originalDirector(request)
			request.Header.Del("X-Forwarded-For")
			request.Header.Del("X-Forwarded-Host")
			request.Header.Del("X-Forwarded-Proto")
			request.Header.Del("X-Real-IP")
			request.Header.Set("X-Forwarded-Host", originalHost)
			request.Header.Set("X-Forwarded-Proto", requestProto(request))
			request.Header.Set("X-Real-IP", remoteIP(request.RemoteAddr))
		}
		routes[domain.Hostname] = route{domain: domain, proxy: proxy}
	}
	p.routes.Store(routes)
}

func (p *Proxy) Handler(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if hostPart, _, err := net.SplitHostPort(host); err == nil {
		host = hostPart
	}
	item, ok := p.routes.Load().(map[string]route)[strings.ToLower(host)]
	if !ok {
		http.Error(w, "domínio não configurado", http.StatusNotFound)
		return
	}
	if item.domain.RedirectHTTPS && r.TLS == nil && item.domain.HTTPSEnabled {
		http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusPermanentRedirect)
		return
	}
	item.proxy.ServeHTTP(w, r)
}

func requestProto(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}
func remoteIP(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return address
}

func Sorted(domains []Domain) []Domain {
	result := append([]Domain(nil), domains...)
	sort.Slice(result, func(i, j int) bool { return result[i].Hostname < result[j].Hostname })
	return result
}
