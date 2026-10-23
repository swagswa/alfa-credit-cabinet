import type { components } from './schema.gen';

type Status = components['schemas']['ApplicationStatus'];
export const applicationGroups = {
  response: { label: 'Нужен ваш ответ', statuses: ['need_info', 'accepted', 'signing'] },
  offers: { label: 'Есть предложение', statuses: ['approved', 'counter_offer'] },
  review: { label: 'На рассмотрении', statuses: ['submitted', 'checking', 'scoring', 'manual_review'] },
  drafts: { label: 'Черновики', statuses: ['draft'] },
  completed: { label: 'Завершены', statuses: ['issued', 'closed', 'rejected', 'expired', 'cancelled_by_client'] },
} satisfies Record<string, { label: string; statuses: Status[] }>;
export type ApplicationGroup = keyof typeof applicationGroups;
export const isApplicationGroup = (value: string | null): value is ApplicationGroup => !!value && value in applicationGroups;
export type DashboardSnapshot = {
  page: components['schemas']['ApplicationPage'];
  attention: components['schemas']['ApplicationSummary'][];
  counts: Record<ApplicationGroup, number>;
};
export const defaultApplicationKey = JSON.stringify(['applications', false, 0, null, null, null, '-created_at']);
