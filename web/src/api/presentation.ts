import type { components } from './schema.gen';

type S = components['schemas'];

const currency = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB' });
const rounded = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB', maximumFractionDigits: 0 });
export const money = (kopecks: number, round = false) => (round ? rounded : currency).format(kopecks / 100);
// в полях ввода рубли, в api копейки
export const kop = (rubles: string) => Math.round(Number(rubles) * 100);

export const options = <K extends string>(map: Record<K, string>) =>
  (Object.entries(map) as [K, string][]).map(([key, content]) => ({ key, content }));

export const labels: Record<S['ApplicationStatus'], string> = {
  draft: 'Черновик',
  submitted: 'Заявка отправлена',
  checking: 'Проверяем сведения',
  need_info: 'Нужно дополнить сведения',
  scoring: 'Оцениваем заявку',
  approved: 'Одобрено',
  counter_offer: 'Предлагаем другие условия',
  manual_review: 'Рассматривает сотрудник',
  rejected: 'Отказ',
  accepted: 'Условия приняты',
  signing: 'Ожидает подписания',
  issued: 'Деньги выданы',
  closed: 'Завершено',
  cancelled_by_client: 'Отменена',
  expired: 'Срок истёк',
};
export const products: Record<S['ProductCode'], string> = {
  working_capital: 'Оборотный кредит',
  overdraft: 'Овердрафт',
  equipment: 'Кредит на оборудование',
};
export const goals: Record<S['Goal'], string> = {
  purchase: 'Закупка товара и сырья',
  cash_gap: 'Оплата расходов до поступления денег',
  equipment: 'Оборудование и транспорт',
  renovation: 'Ремонт и открытие точки',
  other: 'Другая цель',
};
export const representatives: Record<S['RepresentativeRole'], string> = {
  director: 'Руководитель компании',
  owner_ip: 'Индивидуальный предприниматель',
  proxy: 'По доверенности',
};
export const kinds: Record<S['DocumentKind'], string> = {
  passport: 'Паспорт',
  power_of_attorney: 'Доверенность',
  statement: 'Выписка по счёту',
  financials: 'Финансовая отчётность',
  supply_contract: 'Договор поставки',
  tax_receipt: 'Квитанция о сдаче декларации',
  loan_agreement: 'Кредитный договор',
};
export const actors: Record<S['ActorRole'], string> = {
  client: 'Клиент',
  manager: 'Менеджер',
  underwriter: 'Андеррайтер',
  admin: 'Администратор',
  system: 'Система',
};
export const fieldNames: Record<string, string> = {
  inn: 'ИНН',
  period_from: 'Начало периода',
  period_to: 'Конец периода',
  year: 'Отчётный год',
  opening_balance_kop: 'Остаток на начало',
  closing_balance_kop: 'Остаток на конец',
  turnover_in_kop: 'Поступления',
  turnover_out_kop: 'Списания',
  revenue_kop: 'Выручка',
  net_profit_kop: 'Чистая прибыль',
  liabilities_long_kop: 'Долгосрочные обязательства',
  liabilities_short_kop: 'Краткосрочные обязательства',
  balance_total_kop: 'Итог баланса',
};
