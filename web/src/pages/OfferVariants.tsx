import { SliderInput } from '@alfalab/core-components/slider-input';
import { Typography } from '@alfalab/core-components/typography';
import { useEffect, useState } from 'react';
import { api, settle } from '../api/client';
import { money } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { useCabinetCache } from '../CabinetCache';

type App = components['schemas']['ApplicationView'];
type Variant = components['schemas']['OfferVariant'];
export type Choice = { amount_kop: number; term_months: number; fit?: Variant['fit'] };

const step = 1_000_000; // 10 тыс. руб., как в анкете

// клиент двигает сумму и срок под встречным, банк пересчитывает той же политикой
export function OfferVariants({ card, onChoice }: { card: App; onChoice: (choice: Choice | null) => void }) {
  const offer = card.offer!;
  const cache = useCabinetCache();
  const [bounds, setBounds] = useState<{ min: number; termMin: number; termMax: number }>();
  const [amount, setAmount] = useState(offer.amount_kop);
  const [term, setTerm] = useState(offer.term_months);
  const [variant, setVariant] = useState<Variant>();
  const [failed, setFailed] = useState(false);
  const own = amount === offer.amount_kop && term === offer.term_months;

  useEffect(
    () =>
      settle(
        cache.request('products', () => api.GET('/products')),
        ({ data }) => {
          const p = data?.find((item) => item.code === card.product);
          if (p) setBounds({ min: p.amount_min_kop, termMin: p.term_min_months, termMax: p.term_max_months });
        },
      ),
    [cache, card.product],
  );
  useEffect(() => {
    if (own) {
      onChoice(null);
      return;
    }
    onChoice({ amount_kop: amount, term_months: term });
    let stop = () => {};
    // считаем, когда ползунок остановился, а не на каждом пикселе; ответ на старое положение не нужен
    const timer = setTimeout(
      () =>
        (stop = settle(
          api.GET('/applications/{id}/variant', {
            params: { path: { id: card.id }, query: { amount_kop: amount, term_months: term } },
          }),
          ({ data }) => {
            setFailed(!data);
            setVariant(data);
            if (data) onChoice({ amount_kop: amount, term_months: term, fit: data.fit });
          },
          () => setFailed(true),
        )),
      300,
    );
    return () => {
      clearTimeout(timer);
      stop();
    };
  }, [card.id, amount, term, own, onChoice]);

  if (!bounds) return null;
  const verdict =
    own || !variant
      ? null
      : {
          auto: 'Останетесь в рамках автоматического одобрения, можно принимать.',
          limited: `Столько банк не одобрит: на ${variant.terms?.term_months} мес. до ${money(variant.terms?.amount_kop ?? 0)}.`,
          manual: 'Такие условия решает специалист банка, для этого нужна новая заявка.',
          rejected: 'На таких условиях банк не одобрит.',
        }[variant.fit];
  const payment = own
    ? offer.monthly_payment_kop
    : variant?.fit === 'auto'
      ? variant.terms?.monthly_payment_kop
      : undefined;
  const ownFunds = (card.equipment_cost_kop ?? 0) - amount;
  return (
    <section className="offer-variants" aria-label="Подобрать свои условия">
      <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
        Подобрать свои условия
      </Typography.Title>
      <SliderInput
        block
        label="Сумма, ₽"
        value={amount / 100}
        sliderValue={amount / 100}
        min={bounds.min / 100}
        max={card.amount_kop / 100}
        step={step / 100}
        steps={[money(bounds.min, true), money(card.amount_kop, true)]}
        onChange={(_, { value }) => value !== '' && setAmount(Math.round((value * 100) / step) * step)}
      />
      <SliderInput
        block
        label="Срок, мес."
        value={term}
        sliderValue={term}
        min={bounds.termMin}
        max={bounds.termMax}
        step={1}
        steps={[`${bounds.termMin} мес.`, `${bounds.termMax} мес.`]}
        onChange={(_, { value }) => value !== '' && setTerm(value)}
      />
      <div className="offer-variant-result" role="status" aria-live="polite">
        <Typography.Text tag="p" view="primary-medium">
          Платёж в месяц: <strong>{payment !== undefined ? money(payment) : '-'}</strong>
        </Typography.Text>
        {own && (
          <Typography.Text tag="p" view="primary-small">
            Это предложение банка. Подвигайте сумму или срок, чтобы посмотреть свой вариант.
          </Typography.Text>
        )}
        {verdict && (
          <Typography.Text tag="p" view="primary-small">
            {verdict}
          </Typography.Text>
        )}
        {failed && !own && (
          <Typography.Text tag="p" view="primary-small">
            Не удалось пересчитать. Проверьте сумму и срок.
          </Typography.Text>
        )}
        {!own && card.product === 'equipment' && ownFunds > 0 && (
          <Typography.Text tag="p" view="primary-small">
            Своих денег на оборудование понадобится {money(ownFunds)}.
          </Typography.Text>
        )}
      </div>
    </section>
  );
}
