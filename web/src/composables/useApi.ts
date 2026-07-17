import type { ApiError } from "../types";

export class RequestError extends Error {
  code: string;
  fields?: Record<string, string>;
  status: number;
  constructor(error: ApiError, status: number) {
    super(error.message);
    this.code = error.code;
    this.fields = error.fields;
    this.status = status;
  }
}

export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  if (response.status === 204) return undefined as T;
  const body = await response.json().catch(() => ({}));
  if (!response.ok) {
    const error = body.error || {
      code: "request_failed",
      message: "Não foi possível concluir a operação.",
    };
    throw new RequestError(error, response.status);
  }
  return body as T;
}
