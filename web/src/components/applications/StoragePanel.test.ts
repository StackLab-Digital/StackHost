import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import StoragePanel from "./StoragePanel.vue";

function response(body: unknown, status = 200) { return { ok: status >= 200 && status < 300, status, json: async () => body, blob: async () => new Blob(["backup"]) }; }

describe("StoragePanel", () => {
  afterEach(() => vi.restoreAllMocks());

  it("loads volumes, opens folders and previews files", async () => {
    const fetch = vi.fn(async (url: string) => {
      if (url.endsWith("/storage")) return response({ volumes: [{ name: "uploads", type: "named", in_use: true }] });
      if (url.includes("/backup")) return response({ backups: [] });
      if (url.includes("/file?")) return response({ name: "logo.txt", path: "avatars/logo.txt", content: "hello", size_bytes: 5, modified: "2026-07-18T12:00:00Z" });
      if (url.includes("path=avatars")) return response({ entries: [{ name: "logo.txt", path: "avatars/logo.txt", type: "file", size_bytes: 5 }] });
      return response({ entries: [{ name: "avatars", path: "avatars", type: "directory" }] });
    });
    vi.stubGlobal("fetch", fetch);
    const wrapper = mount(StoragePanel, { props: { applicationId: 1 } });
    await flushPromises();
    expect(wrapper.text()).toContain("uploads");
    await wrapper.get(".storage-entry").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("avatars");
    await wrapper.get(".storage-entry").trigger("click");
    await flushPromises();
    expect(wrapper.text()).toContain("hello");
    expect(wrapper.text()).toContain("avatars");
  });

  it("exposes the upload and new-folder actions", async () => {
    vi.stubGlobal("fetch", vi.fn(async (url: string) => url.endsWith("/storage") ? response({ volumes: [{ name: "data", type: "bind", in_use: true }] }) : response({ backups: [], entries: [] })));
    const wrapper = mount(StoragePanel, { props: { applicationId: 7 } });
    await flushPromises();
    expect(wrapper.text()).toContain("Upload");
    expect(wrapper.text()).toContain("Nova pasta");
    expect(wrapper.text()).toContain("Bind Mount");
  });

  it("renders an image preview from the bounded file response", async () => {
    vi.stubGlobal("fetch", vi.fn(async (url: string) => {
      if (url.endsWith("/storage")) return response({ volumes: [{ name: "media", type: "named", in_use: true }] });
      if (url.includes("/backup")) return response({ backups: [] });
      if (url.includes("/file?")) return response({ name: "logo.png", path: "logo.png", type: "file", mime: "image/png", content: "", content_base64: "aGVsbG8=", size_bytes: 5, modified: "2026-07-18T12:00:00Z" });
      return response({ entries: [{ name: "logo.png", path: "logo.png", type: "file", size_bytes: 5 }] });
    }));
    const wrapper = mount(StoragePanel, { props: { applicationId: 8 } }); await flushPromises(); await wrapper.get(".storage-entry").trigger("click"); await flushPromises(); expect(wrapper.find("img.storage-image-preview").exists()).toBe(true);
  });
});
