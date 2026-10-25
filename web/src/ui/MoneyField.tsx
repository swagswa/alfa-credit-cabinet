import { AmountInput } from '@alfalab/core-components/amount-input';
import { kop } from '../api/presentation';

// Форма хранит рубли строкой, AmountInput и API работают в копейках.
export function MoneyField({
  value,
  onChange,
  ...props
}: {
  value: string;
  onChange: (rubles: string) => void;
  label: string;
  hint?: string;
  error?: string;
  required?: boolean;
  disabled?: boolean;
  positiveOnly?: boolean;
}) {
  return (
    <AmountInput
      {...props}
      className="money-field"
      inputClassName="money-field-input"
      suffix={null}
      block
      bold={false}
      integerLength={12}
      value={value === '' ? null : kop(value)}
      onChange={(_, { value }) => onChange(value === null ? '' : String(value / 100))}
    />
  );
}
