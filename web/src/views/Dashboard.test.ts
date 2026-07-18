import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import Dashboard from "./Dashboard.vue";

class EventSourceStub {
  static instance: EventSourceStub;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onopen: (() => void) | null = null;
  close = vi.fn();
  constructor() {
    EventSourceStub.instance = this;
  }
}

const dashboard = {
  projects: 0,
  applications: 0,
  activity: [],
  infrastructure: { available: false, message: "Docker indisponível." },
};

describe("Dashboard", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("renderiza o estado vazio e o diagnóstico do Docker", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue({
          ok: true,
          json: async () => dashboard,
        }),
    );
    vi.stubGlobal("EventSource", EventSourceStub);
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: { template: "<a><slot /></a>" } } },
    });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(wrapper.text()).toContain("Novo projeto");
    expect(wrapper.text()).toContain("Docker indisponível.");
  });

  it("filtra eventos SSE e ativa polling sem trocar o conteúdo por skeleton", async () => {
    const fetchMock = vi.fn().mockImplementation(async (url: string) => ({
      ok: true,
      status: 200,
      json: async () => (url.endsWith("/me") ? { name: "Admin" } : dashboard),
    }));
    const interval = vi.spyOn(window, "setInterval");
    vi.stubGlobal("fetch", fetchMock);
    vi.stubGlobal("EventSource", EventSourceStub);
    const wrapper = mount(Dashboard, {
      global: { stubs: { RouterLink: { template: "<a><slot /></a>" } } },
    });
    await flushPromises();
    expect(fetchMock).toHaveBeenCalledTimes(2);

    EventSourceStub.instance.onmessage?.(
      new MessageEvent("message", {
        data: JSON.stringify({ event: "connected" }),
      }),
    );
    EventSourceStub.instance.onmessage?.(
      new MessageEvent("message", {
        data: JSON.stringify({
          event: "infrastructure.updated",
          data: { available: true, containers: 1, running: 1 },
        }),
      }),
    );
    await flushPromises();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(wrapper.text()).toContain("Docker conectado");

    EventSourceStub.instance.onmessage?.(
      new MessageEvent("message", {
        data: JSON.stringify({ event: "project.created" }),
      }),
    );
    expect(wrapper.find(".skeleton-card").exists()).toBe(false);
    await flushPromises();
    expect(fetchMock).toHaveBeenCalledTimes(3);

    EventSourceStub.instance.onerror?.();
    expect(interval).toHaveBeenCalledWith(expect.any(Function), 30_000);
    wrapper.unmount();
    interval.mockRestore();
  });
});
