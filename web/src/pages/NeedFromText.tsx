import { Textarea } from '@alfalab/core-components/textarea';
import { Typography } from '@alfalab/core-components/typography';
import { useEffect, useState } from 'react';
import { api, settle } from '../api/client';
import type { components } from '../api/schema.gen';
import { useCabinetCache } from '../CabinetCache';
import { Button } from '../motion';

type Need = { goal: components['schemas']['Goal']; amount_kop?: number; term_months?: number };

// «нужно 2 млн на кофемашину, отдам за пару лет»: модель достаёт цель, сумму и срок,
// продукт подбирают наши правила. Без профиля ai поля нет
export function NeedFromText({ disabled, onNeed }: { disabled: boolean; onNeed: (need: Need) => void }) {
  const cache = useCabinetCache();
  const [ai, setAi] = useState(false);
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState('');
  useEffect(
    () =>
      settle(
        cache.request('health', () => api.GET('/health')),
        ({ data, error }) => setAi(!!(data ?? error)?.ai),
      ),
    [cache],
  );
  if (!ai) return null;
  async function fill() {
    setBusy(true);
    setNote('');
    try {
      const { data } = await api.POST('/products/need', { body: { text: text.trim() } });
      if (!data) {
        setNote('Не получилось разобрать. Выберите цель и сумму в полях ниже.');
        return;
      }
      onNeed(data);
      setNote(
        data.amount_kop && data.term_months
          ? 'Заполнили цель, сумму и срок. Проверьте их ниже.'
          : 'Заполнили, что поняли. Проверьте поля ниже и допишите остальное.',
      );
    } catch {
      setNote('Нет связи с сервером. Заполните поля ниже.');
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="need-from-text" aria-label="Описать своими словами">
      <Textarea
        block
        label="Опишите своими словами"
        // подпись библиотеки к полю не привязана, без этого его имя для скринридера это плейсхолдер
        aria-label="Опишите своими словами"
        placeholder="Например: нужно 2 млн на кофемашину и ремонт зала, отдам за пару лет"
        value={text}
        maxLength={500}
        onChange={(event) => setText(event.target.value)}
        disabled={disabled || busy}
      />
      <Button view="secondary" size={40} loading={busy} disabled={disabled || text.trim().length < 3} onClick={fill}>
        Заполнить
      </Button>
      {note && (
        <Typography.Text tag="p" view="primary-small" role="status">
          {note}
        </Typography.Text>
      )}
    </section>
  );
}
