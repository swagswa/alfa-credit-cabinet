import type { components } from './schema.gen';
export const labels: Record<components['schemas']['ApplicationStatus'], string> = {
  draft: 'Черновик', submitted: 'Заявка отправлена', checking: 'Проверяем сведения', need_info: 'Нужно дополнить сведения', scoring: 'Оцениваем заявку', approved: 'Одобрено', counter_offer: 'Предлагаем другие условия', manual_review: 'Рассматривает сотрудник', rejected: 'Отказ', accepted: 'Условия приняты', signing: 'Ожидает подписания', issued: 'Деньги выданы', closed: 'Завершено', cancelled_by_client: 'Отменена', expired: 'Срок истёк',
};
