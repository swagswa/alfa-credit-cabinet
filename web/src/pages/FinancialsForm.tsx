import { MoneyField } from '../ui/MoneyField';
import { Typography } from '@alfalab/core-components/typography';
import { InlineNotice } from '../ui/InlineNotice';
import { notify } from '../BottomNotice';
import { useState, type FormEvent } from 'react';
import { Input } from '@alfalab/core-components/input';
import { Button } from '../motion';
import { api, errorText } from '../api/client';
import { kop } from '../api/presentation';
import type { components } from '../api/schema.gen';
const fields = {
  revenue_kop: 'Выручка, ₽ (строка 2110)',
  net_profit_kop: 'Чистая прибыль, ₽ (строка 2400)',
  liabilities_long_kop: 'Долгосрочные обязательства, ₽ (строка 1400)',
  liabilities_short_kop: 'Краткосрочные обязательства, ₽ (строка 1500)',
  balance_total_kop: 'Итог баланса, ₽ (строка 1600)',
};
export function FinancialsForm({
  documentId,
  inn,
  refresh,
  initial,
  problems = {},
}: {
  documentId: string;
  inn: string;
  refresh: () => void;
  initial?: components['schemas']['FinancialsValues'];
  // что не сошлось в распознанном, по имени поля
  problems?: Record<string, string>;
}) {
  const [year, setYear] = useState(initial ? String(initial.year) : '');
  const [values, setValues] = useState<Record<string, string>>(
    initial
      ? Object.fromEntries(Object.keys(fields).map((key) => [key, String(initial[key as keyof typeof fields] / 100)]))
      : {},
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const amounts = Object.fromEntries(Object.keys(fields).map((key) => [key, kop(values[key])]));
    if (
      Object.values(amounts).some((value) => !Number.isSafeInteger(value)) ||
      Math.abs(amounts.net_profit_kop) > amounts.revenue_kop ||
      amounts.balance_total_kop < amounts.liabilities_long_kop + amounts.liabilities_short_kop
    ) {
      setError('Проверьте прибыль, выручку и сумму обязательств относительно баланса.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      const financials: components['schemas']['FinancialsValues'] = {
        inn,
        year: Number(year),
        revenue_kop: amounts.revenue_kop,
        net_profit_kop: amounts.net_profit_kop,
        liabilities_long_kop: amounts.liabilities_long_kop,
        liabilities_short_kop: amounts.liabilities_short_kop,
        balance_total_kop: amounts.balance_total_kop,
      };
      const { data, error } = await api.POST('/documents/{id}/confirm', {
        params: { path: { id: documentId } },
        body: { financials },
      });
      if (data) {
        notify('Значения подтверждены.');
        refresh();
      } else setError(errorText(error, 'Не удалось подтвердить значения.'));
    } catch {
      setError('Нет связи с сервером. Попробуйте ещё раз.');
    } finally {
      setBusy(false);
    }
  }
  return (
    <form onSubmit={submit}>
      <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
        Подтвердите итоги отчётности
      </Typography.Title>
      {initial && (
        <Typography.Text tag="p" view="primary-small" className="extracted">
          Поля заполнены из документа. Проверьте каждое значение и при необходимости исправьте.
        </Typography.Text>
      )}
      <Typography.Text tag="p" view="primary-small">
        Перенесите значения из файла. В расчёт попадут только подтверждённые данные. ИНН: {inn}.
      </Typography.Text>
      <Input
        block
        label="Отчётный год"
        type="number"
        min={new Date().getFullYear() - 2}
        max={new Date().getFullYear() - 1}
        required
        error={problems.year}
        value={year}
        onChange={(event) => setYear(event.target.value)}
        disabled={busy}
      />
      {Object.entries(fields).map(([key, label]) => (
        <MoneyField
          key={key}
          label={label}
          positiveOnly={key !== 'net_profit_kop'}
          required
          error={problems[key]}
          value={values[key] ?? ''}
          onChange={(value) => setValues({ ...values, [key]: value })}
          disabled={busy}
        />
      ))}
      {error && <InlineNotice>{error}</InlineNotice>}
      <Button type="submit" loading={busy}>
        Подтвердить отчётность
      </Button>
    </form>
  );
}
