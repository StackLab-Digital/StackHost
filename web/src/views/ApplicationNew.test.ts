import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { describe, expect, it } from "vitest";
import ApplicationNew from "./ApplicationNew.vue";

async function mountPage() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: "/projects/:projectId/applications/new",
        component: ApplicationNew,
      },
      { path: "/projects/:projectId", component: { template: "<div>Projeto</div>" } },
    ],
  });
  await router.push("/projects/1/applications/new");
  await router.isReady();
  const wrapper = mount({ template: "<RouterView />" }, {
    global: {
      plugins: [router],
      stubs: { ComposeCodeEditor: { template: "<div class='cm-editor' />" } },
    },
  });
  await flushPromises();
  return { router, wrapper };
}

describe("ApplicationNew", () => {
  it("renders as a page workspace without a modal or overlay", async () => {
    const { wrapper } = await mountPage();
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
    expect(wrapper.find(".modal-backdrop").exists()).toBe(false);
    expect(wrapper.text()).toContain("Nova aplicação");
    expect(wrapper.text()).toContain("Continuar");

    await wrapper.get('input[placeholder="Ex.: Frontend"]').setValue("Demo");
    await wrapper.get("button.primary").trigger("click");
    expect(wrapper.text()).toContain("Como esta aplicação será publicada?");
  });

  it("descarta a configuração e libera a navegação confirmada", async () => {
    const { router, wrapper } = await mountPage();
    await wrapper.get('input[placeholder="Ex.: Frontend"]').setValue("Demo");

    await router.push("/projects/1");
    await flushPromises();
    expect(router.currentRoute.value.path).toBe(
      "/projects/1/applications/new",
    );
    expect(wrapper.text()).toContain("Descartar configuração?");

    await wrapper.get(".wizard-confirm .danger").trigger("click");
    await flushPromises();
    expect(router.currentRoute.value.path).toBe("/projects/1");
    expect(wrapper.text()).toContain("Projeto");
  });
});
