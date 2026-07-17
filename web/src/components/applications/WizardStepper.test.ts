import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import WizardStepper from "./WizardStepper.vue";

describe("WizardStepper", () => {
  it("marks the current step and completed steps semantically", () => {
    const wrapper = mount(WizardStepper, { props: { step: 2 } });
    expect(wrapper.get('[aria-current="step"]').text()).toContain("2. Origem");
    expect(wrapper.findAll(".done")).toHaveLength(1);
    expect(wrapper.findAll(".active")).toHaveLength(1);
  });
});
