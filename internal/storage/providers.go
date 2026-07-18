package storage

import "context"

// StandaloneStorage and SwarmStorage bind the provider boundary to the
// runtime mode. The Docker adapter remains responsible for the daemon calls;
// these types prevent callers from accidentally mixing discovery strategies.
type StandaloneStorage struct{ Backend StorageManager }
type SwarmStorage struct{ Backend StorageManager }

func (s StandaloneStorage) ListVolumes(ctx context.Context, app string) ([]Volume, error) {
	return s.Backend.ListVolumes(ctx, app, "standalone")
}
func (s StandaloneStorage) Browse(ctx context.Context, app, volume, path string) ([]Entry, error) {
	return s.Backend.Browse(ctx, app, "standalone", volume, path)
}
func (s StandaloneStorage) Read(ctx context.Context, app, volume, path string) (File, error) {
	return s.Backend.Read(ctx, app, "standalone", volume, path)
}
func (s StandaloneStorage) Upload(ctx context.Context, app, volume, path string, content []byte, filename string) error {
	return s.Backend.Upload(ctx, app, "standalone", volume, path, content, filename)
}
func (s StandaloneStorage) Delete(ctx context.Context, app, volume, path string) error {
	return s.Backend.Delete(ctx, app, "standalone", volume, path)
}
func (s StandaloneStorage) Backup(ctx context.Context, app, volume string) (string, error) {
	return s.Backend.Backup(ctx, app, "standalone", volume)
}
func (s StandaloneStorage) Restore(ctx context.Context, app, volume, backup string) error {
	return s.Backend.Restore(ctx, app, "standalone", volume, backup)
}

func (s SwarmStorage) ListVolumes(ctx context.Context, app string) ([]Volume, error) {
	return s.Backend.ListVolumes(ctx, app, "swarm")
}
func (s SwarmStorage) Browse(ctx context.Context, app, volume, path string) ([]Entry, error) {
	return s.Backend.Browse(ctx, app, "swarm", volume, path)
}
func (s SwarmStorage) Read(ctx context.Context, app, volume, path string) (File, error) {
	return s.Backend.Read(ctx, app, "swarm", volume, path)
}
func (s SwarmStorage) Upload(ctx context.Context, app, volume, path string, content []byte, filename string) error {
	return s.Backend.Upload(ctx, app, "swarm", volume, path, content, filename)
}
func (s SwarmStorage) Delete(ctx context.Context, app, volume, path string) error {
	return s.Backend.Delete(ctx, app, "swarm", volume, path)
}
func (s SwarmStorage) Backup(ctx context.Context, app, volume string) (string, error) {
	return s.Backend.Backup(ctx, app, "swarm", volume)
}
func (s SwarmStorage) Restore(ctx context.Context, app, volume, backup string) error {
	return s.Backend.Restore(ctx, app, "swarm", volume, backup)
}
