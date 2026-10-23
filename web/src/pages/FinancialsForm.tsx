import { notify } from '../BottomNotice';
import { useState, type FormEvent } from 'react';
import { Input } from '@alfalab/core-components/input';
import { Button } from '../motion';
import { api } from '../api/client';
import { kop } from '../api/presentation';
import type { components } from '../api/schema.gen';
const fields = {
  revenue_kop: 'Выручка, ₽ (строка 2110)', net_profit_kop: 'Чистая прибыль, ₽ (строка 2400)',
  liabilities_long_kop: 'Долгосрочные обязательства, ₽ (строка 1400)', liabilities_short_kop: 'Краткосрочные обязательства, ₽ (строка 1500)', balance_total_kop: 'Итог баланса, ₽ (строка 1600)',
};
export function FinancialsForm({ documentId, inn, refresh, initial }: { documentId: string; inn: string; refresh: () => void; initial?: components["schemas"]["FinancialsValues"] }) {
  const [year, setYear] = useState(initial ? String(initial.year) : '');
  const [values, setValues] = useState<Record<string, string>>(initial ? Object.fromEntries(Object.keys(fields).map(key => [key, String(initial[key as keyof typeof fields] / 100)])) : {});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (busy) return;
    const amounts = Object.fromEntries(Object.keys(fields).map((key) => [key, kop(values[key])]));
    if (Object.values(amounts).some((value) => !Number.isSafeInteger(value)) || Math.abs(amounts.net_profit_kop) > amounts.revenue_kop || amounts.balance_total_kop < amounts.liabilities_long_kop + amounts.liabilities_short_kop) { setError('Проверьте прибыль, выручку и сумму обязательств относительно баланса.'); return; }
    setBusy(true); setError('');
    try {
      const financials: components['schemas']['FinancialsValues'] = {
        inn, year: Number(year), revenue_kop: amounts.revenue_kop, net_profit_kop: amounts.net_profit_kop,
        liabilities_long_kop: amounts.liabilities_long_kop, liabilities_short_kop: amounts.liabilities_short_kop, balance_total_kop: amounts.balance_total_kop,
      };
      const { data, error } = await api.POST('/documents/{id}/confirm', { params: { path: { id: documentId } }, body: { financials } });
      if (data) { notify('Значения подтверждены.'); refresh(); } else setError(error?.title ?? 'Не удалось подтвердить значения.');
    } catch { setError('Нет связи с сервером. Попробуйте ещё раз.'); }
    finally { setBusy(false); }
  }
  return <form onSubmit={submit}>
    <h3>Подтвердите итоги отчётности</h3>{initial && <p className="extracted">Поля заполнены из документа. Проверьте каждое значение и при необходимости исправьте.</p>}<p>Перенесите значения из файла. В расчёт попадут только подтверждённые данные. ИНН: {inn}.</p>
    <Input block label="Отчётный год" type="number" min={new Date().getFullYear() - 2} max={new Date().getFullYear() - 1} required value={year} onChange={(event) => setYear(event.target.value)} disabled={busy} />
    {Object.entries(fields).map(([key, label]) => <Input block key={key} label={label} type="number" step="0.01" min={key === 'net_profit_kop' ? undefined : 0} required value={values[key] ?? ''} onChange={(event) => setValues({ ...values, [key]: event.target.value })} disabled={busy} />)}
    {error && <p role="alert">{error}</p>}
    <Button type="submit" loading={busy}>Подтвердить отчётность</Button>
  </form>;
}
