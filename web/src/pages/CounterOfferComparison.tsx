import { Typography } from '@alfalab/core-components/typography';
import { useEffect, useState } from 'react';
import { api, settle } from '../api/client';
import { money, products, rate } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { useCabinetCache } from '../CabinetCache';

type App = components['schemas']['ApplicationView'];
type Baseline = components['schemas']['RecommendationItem'];

export function CounterOfferComparison({ card }: { card: App }) {
  const cache = useCabinetCache();
  const [baseline, setBaseline] = useState<Baseline>();
  const [loading, setLoading] = useState(true);
  const { product, purpose, amount_kop, term_months, monthly_revenue_kop } = card;
  useEffect(
    () =>
      settle(
        cache.request(
          JSON.stringify(['comparison', product, purpose, amount_kop, term_months, monthly_revenue_kop]),
          () =>
            api.GET('/products/recommendation', {
              params: {
                query: {
                  goal: purpose,
                  amount_kop,
                  term_months,
                  ...(monthly_revenue_kop !== undefined ? { monthly_revenue_kop } : {}),
                },
              },
            }),
        ),
        ({ data }) => {
          setBaseline(
            data?.items?.find(
              (item) => item.product === product && item.amount_kop === amount_kop && item.term_months === term_months,
            ),
          );
          setLoading(false);
        },
        () => setLoading(false),
      ),
    [cache, product, purpose, amount_kop, term_months, monthly_revenue_kop],
  );
  if (!card.offer) return null;
  const offer = card.offer;
  const rows = [
    ['Продукт', products[card.product], products[offer.product]],
    ['Сумма', money(card.amount_kop), money(offer.amount_kop)],
    ['Срок', `${card.term_months} мес.`, `${offer.term_months} мес.`],
    [
      'Ставка в год',
      baseline ? rate(baseline.rate_bp) : loading ? 'Считаем ориентир…' : 'Ориентир недоступен',
      rate(offer.rate_bp),
    ],
    [
      card.product === 'overdraft' || offer.product === 'overdraft' ? 'Платёж / проценты в месяц' : 'Платёж в месяц',
      baseline ? money(baseline.monthly_payment_kop) : loading ? 'Считаем ориентир…' : 'Ориентир недоступен',
      money(offer.monthly_payment_kop),
    ],
  ];
  const ownFunds = Math.max(0, (card.equipment_cost_kop ?? 0) - offer.amount_kop);
  return (
    <section className="counter-comparison" aria-label="Сравнение условий">
      <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
        Что изменилось
      </Typography.Title>
      <div className="counter-comparison-grid" role="table" aria-label="Запрос и предложение банка">
        <div className="counter-comparison-row counter-comparison-head" role="row">
          <span role="columnheader">Параметр</span>
          <strong role="columnheader">Ваш запрос</strong>
          <strong role="columnheader">Банк предлагает</strong>
        </div>
        {rows.map(([name, requested, offered]) => (
          <div className="counter-comparison-row" role="row" key={name}>
            <span role="rowheader">{name}</span>
            <span role="cell">{requested}</span>
            <strong role="cell">{offered}</strong>
          </div>
        ))}
      </div>
      <Typography.Text tag="p" view="primary-small" className="hint">
        Ставка и платёж в колонке «Ваш запрос» рассчитаны для сравнения по текущей базовой ставке продукта.
        {(card.product === 'overdraft' || offer.product === 'overdraft') &&
          ' Для овердрафта показаны проценты за месяц при полном использовании лимита.'}
      </Typography.Text>
      {(offer.product === 'equipment' || card.product === 'equipment') && card.equipment_cost_kop !== undefined && (
        <div className="counter-own-funds">
          <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
            {ownFunds > 0 ? 'Сколько потребуется своих денег' : 'Стоимость покупки покрыта предложенной суммой'}
          </Typography.Title>
          {ownFunds > 0 && <strong>{money(ownFunds)}</strong>}
          <Typography.Text tag="p" view="primary-small">
            Стоимость оборудования - {money(card.equipment_cost_kop)}, банк предлагает {money(offer.amount_kop)}.
          </Typography.Text>
        </div>
      )}
    </section>
  );
}
