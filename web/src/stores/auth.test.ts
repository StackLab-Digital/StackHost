import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAuthStore } from "./auth";

describe("auth store", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal("fetch", vi.fn());
  });
  it("loads the authenticated user", async () => {
    vi.mocked(fetch).mockResolvedValue({
      ok: true,
      json: async () => ({
        id: 1,
        name: "Admin",
        email: "admin@example.com",
        role: "admin",
      }),
    } as Response);
    const store = useAuthStore();
    await store.load();
    expect(store.user?.email).toBe("admin@example.com");
  });
  it("clears an unauthenticated response", async () => {
    vi.mocked(fetch).mockResolvedValue({ ok: false } as Response);
    const store = useAuthStore();
    await store.load();
    expect(store.user).toBeNull();
  });
});
