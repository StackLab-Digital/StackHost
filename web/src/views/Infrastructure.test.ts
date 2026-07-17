import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import Infrastructure from "./Infrastructure.vue";

describe("Infrastructure", () => {
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
});
