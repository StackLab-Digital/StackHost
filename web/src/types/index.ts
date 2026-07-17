export type ApiError = {
  code: string;
  message: string;
  fields?: Record<string, string>;
};
export type Project = {
  id: number;
  name: string;
  slug: string;
  description: string;
  status: string;
  applications_count?: number;
  created_at: string;
  updated_at: string;
};
export type Application = {
  id: number;
  project_id: number;
  name: string;
  slug: string;
  source_type: string;
  docker_stack_name: string;
  status: string;
  created_at: string;
  updated_at: string;
};
export type Infrastructure = {
  available: boolean;
  message?: string;
  engine_version?: string;
  operating_system?: string;
  cpus?: number;
  memory_bytes?: number;
  containers: number;
  running: number;
  images: number;
  volumes: number;
  networks: number;
  stacks: number;
  swarm: {
    active: boolean;
    message?: string;
    nodes: number;
    managers: number;
    workers: number;
    services: number;
    tasks_failed: number;
    tasks_running: number;
    tasks_pending: number;
  };
};
