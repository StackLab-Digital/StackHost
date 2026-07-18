<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { File, Folder, RefreshCw } from "lucide-vue-next";
import { api } from "../../composables/useApi";
import { useToast } from "../../composables/useToast";

type Volume = { name: string; type: string; mountpoint?: string; in_use: boolean; size_bytes?: number; used_bytes?: number };
type Entry = { name: string; path: string; type: "file" | "directory"; size_bytes?: number; modified?: string };
const props = defineProps<{ applicationId: string | number }>();
const toast = useToast();
const volumes = ref<Volume[]>([]);
const selected = ref<Volume | null>(null);
const entries = ref<Entry[]>([]);
const currentPath = ref("");
const file = ref<{ name: string; path: string; type: string; mime?: string; content: string; content_base64?: string; size_bytes: number; modified: string } | null>(null);
const loading = ref(false);
const search = ref("");
const sort = ref<"name" | "type">("name");
const uploadInput = ref<HTMLInputElement | null>(null);
const backups = ref<Array<{ name: string; size_bytes: number; created_at?: string }>>([]);
const error = ref("");
const breadcrumbs = computed(() => currentPath.value ? currentPath.value.split("/") : []);
const visibleEntries = computed(() => entries.value.filter((entry) => entry.name.toLowerCase().includes(search.value.toLowerCase())).sort((a, b) => sort.value === "type" ? a.type.localeCompare(b.type) || a.name.localeCompare(b.name) : a.name.localeCompare(b.name)));
const imageSource = computed(() => file.value?.mime?.startsWith("image/") && file.value.content_base64 ? `data:${file.value.mime};base64,${file.value.content_base64}` : "");
function formatBytes(value = 0) { if (!value) return "—"; const units = ["B", "KB", "MB", "GB", "TB"]; let i = 0; let n = value; while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; } return `${n.toFixed(i ? 1 : 0)} ${units[i]}`; }
async function loadVolumes() {
  loading.value = true; error.value = "";
  try { const result = await api<{ volumes: Volume[] }>(`/api/v1/applications/${props.applicationId}/storage`); volumes.value = result.volumes || []; if (!selected.value && volumes.value[0]) await selectVolume(volumes.value[0]); }
  catch (err) { error.value = err instanceof Error ? err.message : "Não foi possível carregar o storage."; }
  finally { loading.value = false; }
}
async function selectVolume(volume: Volume) { selected.value = volume; currentPath.value = ""; file.value = null; await loadBackups(); await browse(); }
async function loadBackups() { if (!selected.value) return; try { const result = await api<{ backups: Array<{ name: string; size_bytes: number; created_at?: string }> }>(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value.name)}/backup`); backups.value = result.backups || []; } catch { backups.value = []; } }
async function browse(path = currentPath.value) { if (!selected.value) return; loading.value = true; file.value = null; try { const result = await api<{ entries: Entry[] }>(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value.name)}/tree?path=${encodeURIComponent(path)}`); entries.value = result.entries || []; currentPath.value = path; } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível abrir a pasta."); } finally { loading.value = false; } }
async function open(entry: Entry) { if (entry.type === "directory") return browse(entry.path); try { file.value = await api(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value!.name)}/file?path=${encodeURIComponent(entry.path)}`); } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível ler o arquivo."); } }
async function upload(event: Event) { const input = event.target as HTMLInputElement; const selectedFile = input.files?.[0]; if (!selectedFile || !selected.value) return; const body = new FormData(); body.append("file", selectedFile); try { await api(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value.name)}/upload?path=${encodeURIComponent(currentPath.value)}`, { method: "POST", body }); toast.success("Arquivo enviado."); await browse(); } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível enviar o arquivo."); } input.value = ""; }
async function newFolder() { const name = window.prompt("Nome da pasta"); if (!name || !selected.value) return; const path = currentPath.value ? `${currentPath.value}/${name}` : name; try { await api(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value.name)}/folder`, { method: "POST", body: JSON.stringify({ path }) }); toast.success("Pasta criada."); await browse(); } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível criar a pasta."); } }
async function removeEntry() { if (!file.value || !selected.value || !window.confirm(`Excluir ${file.value.name}?`)) return; try { await api(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value.name)}/file?path=${encodeURIComponent(file.value.path)}`, { method: "DELETE" }); file.value = null; toast.success("Arquivo excluído."); await browse(); } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível excluir o arquivo."); } }
async function renameEntry() { if (!file.value || !selected.value) return; const name = window.prompt("Novo nome", file.value.name); if (!name || name === file.value.name) return; const parent = file.value.path.includes("/") ? file.value.path.slice(0, file.value.path.lastIndexOf("/")) : ""; const newPath = parent ? `${parent}/${name}` : name; try { await api(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value.name)}/file`, { method: "POST", body: JSON.stringify({ path: file.value.path, new_path: newPath }) }); toast.success("Item renomeado."); file.value = null; await browse(); } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível renomear o item."); } }
async function backup() { if (!selected.value) return; try { const result = await api<{ name: string; size_bytes: number; created_at?: string }>(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value.name)}/backup`, { method: "POST" }); backups.value.unshift(result); toast.success("Backup criado."); } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível criar o backup."); } }
async function restore() { if (!selected.value || !backups.value[0] || !window.confirm("Esta operação substituirá os arquivos atuais. Continuar?")) return; try { await api(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value.name)}/restore`, { method: "POST", body: JSON.stringify({ backup: backups.value[0].name, confirm: true }) }); toast.success("Backup restaurado."); await browse(); } catch (err) { toast.error(err instanceof Error ? err.message : "Não foi possível restaurar o backup."); } }
async function download() { if (!file.value || !selected.value) return; const response = await fetch(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value.name)}/download?path=${encodeURIComponent(file.value.path)}`, { method: "POST", credentials: "same-origin" }); if (!response.ok) { toast.error("Não foi possível baixar o arquivo."); return; } const blob = await response.blob(); const link = document.createElement("a"); link.href = URL.createObjectURL(blob); link.download = file.value.name; link.click(); URL.revokeObjectURL(link.href); }
async function downloadBackup(backup: { name: string }) { if (!selected.value) return; const response = await fetch(`/api/v1/applications/${props.applicationId}/storage/${encodeURIComponent(selected.value.name)}/backup/download?name=${encodeURIComponent(backup.name)}`, { credentials: "same-origin" }); if (!response.ok) { toast.error("Não foi possível baixar o backup."); return; } const blob = await response.blob(); const link = document.createElement("a"); link.href = URL.createObjectURL(blob); link.download = backup.name; link.click(); URL.revokeObjectURL(link.href); }
watch(() => props.applicationId, loadVolumes, { immediate: true });
</script>

<template>
  <section class="storage-panel">
    <div class="storage-toolbar"><div><p class="kicker">STORAGE</p><h2>Arquivos persistentes</h2></div><div class="storage-actions"><input ref="uploadInput" hidden type="file" @change="upload" /><button class="secondary" type="button" :disabled="!selected" @click="uploadInput?.click()">Upload</button><button class="secondary" type="button" :disabled="!selected" @click="newFolder">Nova pasta</button><button class="secondary" type="button" :disabled="!selected" @click="backup">Backup</button><button class="secondary" type="button" :disabled="!backups.length" @click="restore">Restaurar</button><button class="secondary" type="button" :disabled="loading" @click="loadVolumes"><RefreshCw :size="15" /> Atualizar</button></div></div>
    <p v-if="error" class="inline-error">{{ error }}</p>
    <div v-if="backups.length" class="storage-backups"><strong>Backups recentes</strong><button v-for="backup in backups.slice(0, 3)" :key="backup.name" type="button" @click="downloadBackup(backup)">{{ backup.name }} · {{ formatBytes(backup.size_bytes) }}<small v-if="backup.created_at"> · {{ new Date(backup.created_at).toLocaleString('pt-BR') }}</small></button></div>
    <div v-if="loading && !volumes.length" class="empty"><p>Descobrindo volumes…</p></div>
    <div v-else-if="!volumes.length" class="empty"><p>Nenhum volume encontrado para esta aplicação.</p></div>
    <div v-else class="storage-explorer">
      <aside class="storage-volumes"><button v-for="volume in volumes" :key="volume.name" type="button" :class="{ active: selected?.name === volume.name }" @click="selectVolume(volume)"><strong>{{ volume.name }}</strong><small>{{ volume.type === 'named' ? 'Named Volume' : 'Bind Mount' }}</small><small v-if="volume.size_bytes">{{ formatBytes(volume.used_bytes) }} / {{ formatBytes(volume.size_bytes) }}</small></button></aside>
      <div class="storage-tree"><div class="storage-breadcrumbs"><button type="button" @click="browse('')">{{ selected?.name }}</button><template v-for="(crumb, index) in breadcrumbs" :key="crumb"><span>/</span><button type="button" @click="browse(breadcrumbs.slice(0, index + 1).join('/'))">{{ crumb }}</button></template></div><div class="storage-tree-tools"><input v-model="search" type="search" placeholder="Buscar nesta pasta" /><select v-model="sort" aria-label="Ordenar arquivos"><option value="name">Nome</option><option value="type">Tipo</option></select></div><div v-if="loading" class="empty"><p>Carregando…</p></div><button v-for="entry in visibleEntries" v-else :key="entry.path" class="storage-entry" type="button" @click="open(entry)"><Folder v-if="entry.type === 'directory'" :size="17" /><File v-else :size="17" /><span>{{ entry.name }}</span><small>{{ entry.type === 'file' ? formatBytes(entry.size_bytes) : 'Pasta' }}</small></button><div v-if="!loading && !visibleEntries.length" class="empty compact-empty"><p>Esta pasta está vazia.</p></div></div>
      <aside v-if="file" class="storage-preview"><p class="kicker">ARQUIVO</p><h3>{{ file.name }}</h3><small>{{ file.type }} · {{ formatBytes(file.size_bytes) }} · {{ file.modified }}</small><p class="storage-file-path">{{ file.path }}</p><div class="storage-file-actions"><button class="secondary" type="button" @click="renameEntry">Renomear</button><button class="ghost danger" type="button" @click="removeEntry">Excluir</button><button class="secondary" type="button" @click="download">Baixar</button></div><img v-if="imageSource" class="storage-image-preview" :src="imageSource" :alt="file.name" /><pre v-else>{{ file.content }}</pre></aside>
    </div>
  </section>
</template>

<style scoped>
.storage-panel { display:grid; gap:18px; }
.storage-toolbar { display:flex; align-items:center; justify-content:space-between; gap:16px; }
.storage-toolbar h2 { margin:4px 0 0; }
.storage-toolbar button { display:flex; align-items:center; gap:7px; }
.storage-actions, .storage-file-actions { display:flex; flex-wrap:wrap; justify-content:flex-end; gap:7px; }
.storage-backups { display:flex; flex-wrap:wrap; align-items:center; gap:8px; color:#8e95a3; font-size:12px; }
.storage-backups button { padding:4px 7px; border:1px solid #343842; border-radius:5px; background:transparent; color:#b7c8ff; cursor:pointer; }
.storage-explorer { display:grid; grid-template-columns:190px minmax(260px,1fr) minmax(220px,.8fr); min-height:420px; overflow:hidden; border:1px solid #2a2d34; border-radius:10px; background:#15171c; }
.storage-volumes { padding:8px; border-right:1px solid #2a2d34; }
.storage-volumes button { display:grid; width:100%; gap:4px; padding:11px 10px; border:0; border-radius:7px; background:transparent; color:#e6e8ed; text-align:left; cursor:pointer; }
.storage-volumes button.active, .storage-volumes button:hover { background:#262a32; }
.storage-volumes small, .storage-entry small, .storage-preview small { color:#8e95a3; }
.storage-tree { min-width:0; padding:12px; border-right:1px solid #2a2d34; }
.storage-breadcrumbs { display:flex; flex-wrap:wrap; align-items:center; gap:5px; min-height:28px; margin-bottom:9px; color:#8e95a3; font-size:12px; }
.storage-breadcrumbs button { padding:0; border:0; background:none; color:#b7c8ff; cursor:pointer; }
.storage-tree-tools { display:flex; gap:6px; margin-bottom:8px; }
.storage-tree-tools input, .storage-tree-tools select { min-width:0; padding:6px 8px; border:1px solid #343842; border-radius:5px; background:#101216; color:#e6e8ed; font-size:12px; }
.storage-tree-tools input { flex:1; }
.storage-entry { display:grid; grid-template-columns:20px minmax(0,1fr) auto; align-items:center; gap:8px; width:100%; padding:9px 8px; border:0; border-radius:6px; background:transparent; color:#e6e8ed; text-align:left; cursor:pointer; }
.storage-entry:hover { background:#252932; }
.storage-preview { min-width:0; padding:16px; overflow:auto; }
.storage-preview h3 { overflow-wrap:anywhere; margin:5px 0; }
.storage-preview pre { max-height:320px; margin-top:14px; overflow:auto; white-space:pre-wrap; overflow-wrap:anywhere; color:#d7dbe4; font:12px/1.55 ui-monospace,SFMono-Regular,Menlo,monospace; }
.storage-file-path { margin:5px 0; overflow-wrap:anywhere; color:#8e95a3; font:11px ui-monospace,SFMono-Regular,Menlo,monospace; }
.storage-image-preview { display:block; max-width:100%; max-height:320px; margin-top:15px; object-fit:contain; border-radius:6px; background:#0d0f12; }
.inline-error { padding:12px; border:1px solid #633c42; border-radius:8px; background:#2a1c20; color:#efb3b3; }
@media (max-width:900px) { .storage-explorer { grid-template-columns:150px minmax(220px,1fr); } .storage-preview { grid-column:1 / -1; border-top:1px solid #2a2d34; border-right:0; } }
</style>
