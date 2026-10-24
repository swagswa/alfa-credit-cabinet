import { notify } from '../BottomNotice';
import { CalendarInput } from '@alfalab/core-components/calendar-input';
import { useState, type FormEvent } from 'react';
import { Input } from '@alfalab/core-components/input';
import { Button } from '../motion';
import { api } from '../api/client';
import { validDate } from '../api/date';
import { kop } from '../api/presentation';
import type { components } from '../api/schema.gen';
const fields = {
  opening_balance_kop: 'Остаток на начало, ₽',
  turnover_in_kop: 'Поступления за период, ₽',
  turnover_out_kop: 'Списания за период, ₽',
  closing_balance_kop: 'Остаток на конец, ₽',
};
export function StatementForm({
  documentId,
  inn,
  refresh,
  initial,
}: {
  documentId: string;
  inn: string;
  refresh: () => void;
  initial?: components['schemas']['StatementValues'];
}) {
  const [from, setFrom] = useState(initial?.period_from ?? '');
  const [to, setTo] = useState(initial?.period_to ?? '');
  const [values, setValues] = useState<Record<string, string>>(
    initial
      ? Object.fromEntries(Object.keys(fields).map((key) => [key, String(initial[key as keyof typeof fields] / 100)]))
      : {},
  );
  const [monthly, setMonthly] = useState<Record<string, string>>(
    initial ? Object.fromEntries(initial.monthly_in.map((item) => [item.month, String(item.amount_kop / 100)])) : {},
  );
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const months: string[] = [];
  if (validDate(from) && validDate(to) && from < to) {
    const start = new Date(from + 'T00:00:00Z'),
      end = new Date(to + 'T00:00:00Z');
    const date = new Date(Date.UTC(start.getUTCFullYear(), start.getUTCMonth(), 1));
    while (date <= end && months.length < 14) {
      months.push(date.toISOString().slice(0, 7));
      date.setUTCMonth(date.getUTCMonth() + 1);
    }
  }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    const amounts = Object.fromEntries(Object.keys(fields).map((key) => [key, kop(values[key])]));
    const monthly_in = months.map((month) => ({ month, amount_kop: kop(monthly[month]) }));
    if (
      !months.length ||
      months.length > 13 ||
      to > new Date().toISOString().slice(0, 10) ||
      [...Object.values(amounts), ...monthly_in.map((item) => item.amount_kop)].some(
        (value) => !Number.isSafeInteger(value),
      ) ||
      amounts.turnover_in_kop <= 0 ||
      amounts.turnover_out_kop < 0 ||
      monthly_in.some((item) => item.amount_kop < 0) ||
      Math.abs(
        amounts.opening_balance_kop + amounts.turnover_in_kop - amounts.turnover_out_kop - amounts.closing_balance_kop,
      ) > 100 ||
      Math.abs(monthly_in.reduce((sum, item) => sum + item.amount_kop, 0) - amounts.turnover_in_kop) >
        months.length * 100
    ) {
      setError('Проверьте период, остатки и суммы: поступления по месяцам должны совпадать с итогом.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      const { data, error } = await api.POST('/documents/{id}/confirm', {
        params: { path: { id: documentId } },
        body: {
          statement: {
            inn,
            period_from: from,
            period_to: to,
            opening_balance_kop: amounts.opening_balance_kop,
            turnover_in_kop: amounts.turnover_in_kop,
            turnover_out_kop: amounts.turnover_out_kop,
            closing_balance_kop: amounts.closing_balance_kop,
            monthly_in,
          },
        },
      });
      if (data) {
        notify('Значения подтверждены.');
        refresh();
      } else setError(error?.title ?? 'Не удалось подтвердить выписку.');
    } catch {
      setError('Нет связи с сервером. Попробуйте ещё раз.');
    } finally {
      setBusy(false);
    }
  }
  return (
    <form onSubmit={submit}>
      <h3>Подтвердите итоги выписки</h3>
      {initial && (
        <p className="extracted">
          Поля заполнены из документа. Проверьте каждое значение и при необходимости исправьте.
        </p>
      )}
      <p>
        Этот блок нужен только для подтверждения доходов. Перенесите суммы из банковской выписки или нажмите «Заполнить
        из документа», если распознавание завершено. ИНН: {inn}. Период: от 1 до 13 месяцев.
      </p>
      <CalendarInput
        block
        label="Начало периода"
        value={from ? from.split('-').reverse().join('.') : ''}
        onChange={(_, p) => setFrom(p.value.split('.').reverse().join('-'))}
        disabled={busy}
      />
      <CalendarInput
        block
        label="Конец периода"
        value={to ? to.split('-').reverse().join('.') : ''}
        onChange={(_, p) => setTo(p.value.split('.').reverse().join('-'))}
        disabled={busy}
      />
      {Object.entries(fields).map(([key, label]) => (
        <Input
          block
          key={key}
          label={label}
          type="number"
          step="0.01"
          required
          value={values[key] ?? ''}
          onChange={(event) => setValues({ ...values, [key]: event.target.value })}
          disabled={busy}
        />
      ))}
      {months.length <= 13 &&
        months.map((month) => (
          <Input
            block
            key={month}
            label={`Поступления за ${month}, ₽`}
            type="number"
            min={0}
            step="0.01"
            required
            value={monthly[month] ?? ''}
            onChange={(event) => setMonthly({ ...monthly, [month]: event.target.value })}
            disabled={busy}
          />
        ))}
      {error && <p role="alert">{error}</p>}
      <Button type="submit" loading={busy}>
        Подтвердить выписку
      </Button>
    </form>
  );
}
