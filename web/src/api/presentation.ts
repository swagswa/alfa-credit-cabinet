import type { components } from './schema.gen';

type S = components['schemas'];

const currency = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB' });
const rounded = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB', maximumFractionDigits: 0 });
export const money = (kopecks: number, round = false) => (round ? rounded : currency).format(kopecks / 100);
export const rate = (bp: number) => `${(bp / 100).toLocaleString('ru-RU')}%`;
// в полях ввода рубли, в api копейки
export const kop = (rubles: string) => Math.round(Number(rubles) * 100);

// те же правила документов, что у процесса: что банк попросит после подачи, если не приложить сейчас.
// выписку «если мало оборотов» до проверок не узнать, её не требуем
export function requiredDocs(card: S['ApplicationView'], product?: S['Product']): S['DocumentKind'][] {
  const need = (product?.documents ?? [])
    .filter((d) => !d.when || (d.when === 'amount_over' && card.amount_kop > (d.amount_over_kop ?? 0)))
    .map((d) => d.kind);
  return card.representative_role === 'proxy' ? [...need, 'power_of_attorney'] : need;
}

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
  monthly_in: 'Поступления по месяцам',
};

export const sources: Record<components['schemas']['CheckSource'], string> = {
  egrul: 'Реестр компаний и предпринимателей',
  fns: 'Налоговая служба',
  bki: 'Бюро кредитных историй',
  internal: 'Внутренние данные банка',
};

// у мягких стоп-факторов текста для клиента нет, банку объясняем сами
export const bankReasons: Partial<Record<components['schemas']['ReasonCode'], string>> = {
  SF_UNRELIABLE: 'В ЕГРЮЛ есть отметки о недостоверных сведениях',
  SF_MASS_ADDRESS: 'Адрес массовой регистрации',
  SF_MASS_DIRECTOR: 'Руководитель числится во многих компаниях',
  SF_FOREIGN_SHARE: 'Доля иностранных участников больше 25%',
  SF_OWNER_OVERDUE: 'У собственника за год была просрочка больше 30 дней',
  SF_AML_FLAG: 'Высокий риск по проверке на отмывание денег',
  SF_SOURCE_UNAVAILABLE: 'Источник не ответил и после повторных попыток, данных для решения не хватает',
};

const pct = (v: number) => `${Math.round(v * 100)}%`;
const bankHistory = [
  'счёта нет или он младше 3 мес.',
  'счёт 3-11 мес.',
  'счёт больше года',
  'счёт больше года, кредит погашен',
];
// значения входов скоринга в единицах rules/scoring.yaml
const factorUnits: Record<string, (v: number) => string> = {
  F1: (v) => `${v} мес.`,
  F2: (v) => `${v >= 1 ? '+' : ''}${pct(v - 1)} к прошлому полугодию`,
  F3: (v) => `разброс по месяцам ${pct(v)}`,
  F4: (v) => `${pct(v)} выручки`,
  F5: (v) => (v ? `от ${v} дн.` : 'не было'),
  F6: (v) => `${v} шт.`,
  F7: (v) => (v ? money(v * 100, true) : 'нет'),
  F8: (v) => bankHistory[v] ?? String(v),
  F9: (v) => `ОКВЭД ${v}`,
};
export const factorValue = (code: string, v: number) =>
  factorUnits[code]?.(v) ?? v.toLocaleString('ru-RU', { maximumFractionDigits: 2 });
