import { mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { describe, expect, it } from "vitest";
import ApplicationNew from "./ApplicationNew.vue";

describe("ApplicationNew", () => {
  it("renders as a page workspace without a modal or overlay", async () => {
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/projects/:projectId/applications/new", component: ApplicationNew }],
    });
    await router.push("/projects/1/applications/new");
    await router.isReady();
    const wrapper = mount(ApplicationNew, {
      global: {
        plugins: [router],
        stubs: {
          ComposeCodeEditor: { template: "<div class='cm-editor' />" },
        },
      },
    });
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false);
    expect(wrapper.find(".modal-backdrop").exists()).toBe(false);
    expect(wrapper.text()).toContain("Nova aplicação");
    expect(wrapper.text()).toContain("Continuar");

    await wrapper.get('input[placeholder="Ex.: Frontend"]').setValue("Demo");
    await wrapper.get("button.primary").trigger("click");
    expect(wrapper.text()).toContain("Como esta aplicação será publicada?");
  });
});
