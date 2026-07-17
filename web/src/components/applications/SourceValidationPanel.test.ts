import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import SourceValidationPanel from "./SourceValidationPanel.vue";

describe("SourceValidationPanel", () => {
  it("distinguishes valid warnings from errors", () => {
    const warning = mount(SourceValidationPanel, {
      props: { valid: true, warnings: ["Porta não publicada"] },
    });
    expect(warning.text()).toContain("Válido com avisos");
    expect(warning.classes()).toContain("warning");

    const invalid = mount(SourceValidationPanel, {
      props: { valid: false, errors: ["Imagem obrigatória"] },
    });
    expect(invalid.text()).toContain("Revise a configuração");
    expect(invalid.classes()).toContain("invalid");
  });
});
