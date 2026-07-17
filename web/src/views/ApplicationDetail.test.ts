import { mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { describe, expect, it, vi } from "vitest";
import ApplicationDetail from "./ApplicationDetail.vue";

describe("ApplicationDetail", () => {
  it("renderiza a aplicação e a aba de origem sem expor secrets", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(async (url: string) => ({
        ok: true,
        json: async () =>
          url.endsWith("/source")
            ? { source_type: "compose", configured: false, payload: {} }
            : {
                id: 1,
                name: "Frontend",
                description: "Site público",
                slug: "frontend",
                source_type: "compose",
                docker_stack_name: "frontend",
                configuration_status: "draft",
                source_revision: 0,
                project: { id: 2, name: "Website" },
                activity: [],
              },
      })),
    );
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/projects/:id", component: { template: "<div />" } },
        {
          path: "/projects/:projectId/applications/:applicationId",
          component: ApplicationDetail,
        },
      ],
    });
    await router.push("/projects/2/applications/1?tab=source");
    await router.isReady();
    const wrapper = mount(ApplicationDetail, { global: { plugins: [router] } });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(wrapper.text()).toContain("Frontend");
    expect(wrapper.text()).toContain("Docker Compose");
    expect(wrapper.text()).toContain("Salvar alterações");
  });
});
