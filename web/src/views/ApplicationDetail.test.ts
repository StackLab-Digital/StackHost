import { flushPromises, mount } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import ApplicationDetail from "./ApplicationDetail.vue";

type TestDeployment = {
  id: number;
  application_id: number;
  runtime_mode: string;
  status: string;
  source_revision: number;
  stack_name: string;
  trigger_type: string;
  output: string;
  error_code: string;
  error_message: string;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
};

class EventSourceStub {
  static instance: EventSourceStub;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  onerror: (() => void) | null = null;
  onopen: (() => void) | null = null;
  close = vi.fn();
  constructor(readonly url: string) {
    EventSourceStub.instance = this;
  }
}

const application = {
  id: 1,
  name: "Frontend",
  description: "Site público",
  slug: "frontend",
  source_type: "compose",
  docker_stack_name: "frontend",
  configuration_status: "configured",
  source_revision: 7,
  project: { id: 2, name: "Website" },
  activity: [],
};

function deployment(overrides: Partial<TestDeployment> = {}): TestDeployment {
  return {
    id: 11,
    application_id: 1,
    runtime_mode: "standalone",
    status: "succeeded",
    source_revision: 7,
    stack_name: "frontend",
    trigger_type: "manual",
    output: "",
    error_code: "",
    error_message: "",
    created_at: "2026-07-17T12:00:00Z",
    started_at: "2026-07-17T12:00:01Z",
    finished_at: "2026-07-17T12:00:09Z",
    ...overrides,
  };
}

function response(body: unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  };
}

function fetchForDeployments(
  items: TestDeployment[],
  override?: (url: string, options?: RequestInit) => ReturnType<typeof response> | undefined,
) {
  return vi.fn(async (url: string, options?: RequestInit) => {
    const overridden = override?.(url, options);
    if (overridden) return overridden;
    if (url.endsWith("/source")) {
      return response({
        source_type: "compose",
        configured: true,
        payload: { compose_yaml: "services: {}" },
      });
    }
    if (url.endsWith("/runtime")) {
      return response({ mode: "standalone", status: "running", services: [] });
    }
    if (url.endsWith("/deployments")) return response(items);
    return response(application);
  });
}

async function mountDetail(tab: "source" | "deployments" = "deployments") {
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
  await router.push(`/projects/2/applications/1?tab=${tab}`);
  await router.isReady();
  const wrapper = mount(ApplicationDetail, {
    global: {
      plugins: [router],
      stubs: { ComposeCodeEditor: { template: "<div />" } },
    },
  });
  await flushPromises();
  return { router, wrapper };
}

describe("ApplicationDetail", () => {
  afterEach(() => vi.unstubAllGlobals());

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
    const wrapper = mount(ApplicationDetail, {
      global: {
        plugins: [router],
        stubs: { ComposeCodeEditor: { template: "<div />" } },
      },
    });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(wrapper.text()).toContain("Frontend");
    expect(wrapper.text()).toContain("Docker Compose");
    expect(wrapper.text()).toContain("Salvar alterações");
  });

  it("valida a edição atual pelo endpoint de preview", async () => {
    const fetchMock = vi
      .fn()
      .mockImplementation(async (url: string, _options?: RequestInit) => ({
        ok: true,
        status: 200,
        json: async () => {
          if (url.endsWith("/source")) {
            return {
              source_type: "compose",
              configured: true,
              payload: { compose_yaml: "services: {}" },
            };
          }
          if (url.endsWith("/runtime")) {
            return { mode: "standalone", status: "not_deployed", services: [] };
          }
          if (url === "/api/v1/source/validate") {
            return {
              valid: true,
              errors: [],
              warnings: [],
              summary: {
                services: [],
                images: [],
                ports: [],
                volumes: [],
                networks: [],
              },
            };
          }
          return {
            id: 1,
            name: "Frontend",
            description: "Site público",
            slug: "frontend",
            source_type: "compose",
            docker_stack_name: "frontend",
            configuration_status: "configured",
            source_revision: 1,
            project: { id: 2, name: "Website" },
            activity: [],
          };
        },
      }));
    vi.stubGlobal("fetch", fetchMock);
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
    const wrapper = mount(ApplicationDetail, {
      global: {
        plugins: [router],
        stubs: {
          ComposeCodeEditor: {
            props: ["modelValue"],
            emits: ["update:modelValue"],
            template:
              '<textarea data-test="compose" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
          },
        },
      },
    });
    await flushPromises();
    const edited = "services:\n  web:\n    image: nginx:alpine";
    await wrapper.get('[data-test="compose"]').setValue(edited);
    await wrapper
      .findAll("button")
      .find((button) => button.text() === "Validar")!
      .trigger("click");
    await flushPromises();

    const previewCall = fetchMock.mock.calls.find(
      ([url]) => url === "/api/v1/source/validate",
    );
    expect(previewCall).toBeTruthy();
    expect(JSON.parse(previewCall?.[1]?.body as string)).toMatchObject({
      source_type: "compose",
      source: { compose_yaml: edited },
    });
    expect(
      fetchMock.mock.calls.some(([url]) =>
        url.endsWith("/applications/1/source/validate"),
      ),
    ).toBe(false);
  });
});
