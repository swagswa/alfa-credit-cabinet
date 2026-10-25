import { Typography } from '@alfalab/core-components/typography';
import type { components } from '../api/schema.gen';
import { money, rate, products } from '../api/presentation';

type Item = components['schemas']['RecommendationItem'];
export function ProductChoices({
  items,
  selected,
  disabled,
  onSelect,
}: {
  items: Item[];
  selected: components['schemas']['ProductCode'];
  disabled: boolean;
  onSelect: (item: Item) => void;
}) {
  return (
    <fieldset className="product-choices" disabled={disabled}>
      <legend>Выберите вариант финансирования</legend>
      <Typography.Text tag="p" view="primary-small" className="hint">
        Предварительный расчёт. Окончательные условия определит банк после рассмотрения заявки.
      </Typography.Text>
      <div className="product-choice-grid">
        {items.map((item) => (
          <label className="product-choice" key={item.product}>
            <input
              type="radio"
              name="recommended-product"
              checked={selected === item.product}
              onChange={() => onSelect(item)}
              aria-label={`${item.primary ? 'Рекомендация' : 'Альтернатива'}: ${products[item.product]}`}
            />
            <span className="product-choice-body">
              <span className="product-choice-tag">{item.primary ? 'Рекомендация' : 'Альтернатива'}</span>
              <strong className="product-choice-title">{products[item.product]}</strong>
              <span className="product-choice-payment">{money(item.monthly_payment_kop, true)}</span>
              <span>
                {item.product === 'overdraft'
                  ? 'проценты за месяц при полном использовании лимита'
                  : 'ориентир платежа в месяц'}
              </span>
              <span className="product-choice-terms">
                {money(item.amount_kop, true)} · {item.term_months} мес. · {rate(item.rate_bp)} годовых
              </span>
              <span className="product-choice-selection">
                {selected === item.product ? 'Выбран' : 'Выбрать продукт'}
              </span>
            </span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}
