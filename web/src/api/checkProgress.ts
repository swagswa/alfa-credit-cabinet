import type { components } from './schema.gen';

type S = components['schemas'];
export const checkSources = ['egrul', 'fns', 'bki', 'internal'] as const;
export type LiveCheck = S['StreamEvent'] & { source: S['CheckSource'] };
export type CheckProgressState = {
  applicationId: string;
  version: number;
  items: Partial<Record<S['CheckSource'], LiveCheck>>;
};

// SSE is external input. A malformed notification must not break the cabinet.
export function parseCheckEvent(data: string): LiveCheck | undefined {
  try {
    const value = JSON.parse(data);
    if (
      !value ||
      typeof value !== 'object' ||
      typeof value.application_id !== 'string' ||
      !checkSources.includes(value.source) ||
      !Number.isSafeInteger(value.version) ||
      value.version < 0
    )
      return;
    if (value.check_status !== undefined && !['ok', 'not_found', 'unavailable'].includes(value.check_status)) return;
    if (value.attempt !== undefined || value.attempts !== undefined) {
      if (
        !Number.isSafeInteger(value.attempt) ||
        !Number.isSafeInteger(value.attempts) ||
        value.attempt < 1 ||
        value.attempt > value.attempts
      )
        return;
    }
    if (!value.check_status && value.attempt === undefined) return;
    return {
      application_id: value.application_id,
      version: value.version,
      source: value.source,
      check_status: value.check_status,
      attempt: value.attempt,
      attempts: value.attempts,
      error: typeof value.error === 'string' ? value.error.slice(0, 1500) : undefined,
    };
  } catch {
    return;
  }
}

export function updateCheckProgress(state: CheckProgressState, event: LiveCheck): CheckProgressState {
  const current =
    state.applicationId === event.application_id
      ? state
      : {
          applicationId: event.application_id,
          version: 0,
          items: {},
        };
  if (event.version < current.version) return current;
  const previous = current.items[event.source];
  if (
    previous &&
    previous.version === event.version &&
    (previous.check_status || (previous.attempt ?? 0) > (event.attempt ?? Infinity))
  )
    return current;
  return {
    applicationId: event.application_id,
    version: event.version,
    items: { ...current.items, [event.source]: event },
  };
}

export function processLinks(applicationId: string, origin: string) {
  const workflow = new URL(origin);
  workflow.protocol = 'http:';
  workflow.port = '8233';
  workflow.pathname = `/namespaces/default/workflows/${encodeURIComponent(applicationId)}`;
  workflow.search = '';
  workflow.hash = '';
  const trace = new URL(workflow.origin);
  trace.port = '16686';
  trace.pathname = '/search';
  trace.search = new URLSearchParams({
    service: 'worker',
    tags: JSON.stringify({ temporalWorkflowID: applicationId }),
  }).toString();
  return { workflow: workflow.href, trace: trace.href };
}
