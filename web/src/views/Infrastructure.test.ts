import { flushPromises, mount } from "@vue/test-utils";
import { afterEach, describe, expect, it, vi } from "vitest";
import Infrastructure from "./Infrastructure.vue";

describe("Infrastructure", () => {
  afterEach(() => {
    document.body.innerHTML = "";
    vi.unstubAllGlobals();
  });

  it("explica quando o Docker está indisponível", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue({
          ok: true,
          json: async () => ({
            available: false,
            message: "Docker indisponível.",
            swarm: { active: false },
            containers: 0,
            running: 0,
            images: 0,
            volumes: 0,
            networks: 0,
          }),
        }),
    );
    const wrapper = mount(Infrastructure);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(wrapper.text()).toContain("Docker indisponível");
    expect(wrapper.text()).toContain("Docker indisponível.");
  });

  it("executa novamente a preparação ao clicar em tentar novamente", async () => {
    let attempts = 0;
    const fetchMock = vi.fn().mockImplementation(async (url: string) => {
      if (url.endsWith("/swarm/init")) {
        attempts += 1;
        if (attempts === 1) {
          return {
            ok: false,
            status: 500,
            json: async () => ({
              error: { code: "swarm_failed", message: "Falha ao preparar." },
            }),
          };
        }
        return { ok: true, status: 204, json: async () => ({}) };
      }
      return {
        ok: true,
        status: 200,
        json: async () => ({
          available: true,
          swarm: { active: false },
          containers: 0,
          running: 0,
          images: 0,
          volumes: 0,
          networks: 0,
        }),
      };
    });
    vi.stubGlobal("fetch", fetchMock);
    const wrapper = mount(Infrastructure);
    await flushPromises();

    await wrapper
      .findAll("button")
      .find((button) => button.text() === "Preparar ambiente")!
      .trigger("click");
    const prepare = [...document.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Preparar agora",
    );
    prepare?.click();
    await flushPromises();

    const retry = [...document.querySelectorAll("button")].find(
      (button) => button.textContent?.trim() === "Tentar novamente",
    );
    retry?.click();
    await flushPromises();
    expect(attempts).toBe(2);
  });
});
