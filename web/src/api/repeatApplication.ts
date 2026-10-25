import type { components } from './schema.gen';

type App = components['schemas']['ApplicationView'];
export type RepeatApplication = Pick<
  App,
  | 'number'
  | 'company'
  | 'amount_kop'
  | 'term_months'
  | 'product'
  | 'purpose'
  | 'representative_role'
  | 'monthly_revenue_kop'
  | 'equipment_cost_kop'
>;

export function repeatApplication(card: App): { repeatApplication: RepeatApplication } {
  return {
    repeatApplication: {
      number: card.number,
      company: card.company,
      amount_kop: card.amount_kop,
      term_months: card.term_months,
      product: card.product,
      purpose: card.purpose,
      representative_role: card.representative_role,
      monthly_revenue_kop: card.monthly_revenue_kop,
      equipment_cost_kop: card.equipment_cost_kop,
    },
  };
}
