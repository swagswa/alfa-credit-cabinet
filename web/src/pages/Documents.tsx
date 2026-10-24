import { motion, AnimatePresence, Button, ListItem, Select, StatusText, useMotionDisabled } from '../motion';
import { BottomNotice, notify } from '../BottomNotice';
import { useState, type FormEvent } from 'react';
import { Input } from '@alfalab/core-components/input';
import { Attach } from '@alfalab/core-components/attach';
import { api } from '../api/client';
import { kinds, kop } from '../api/presentation';
import { DocumentDetails } from './DocumentDetails';
import type { components } from '../api/schema.gen';

type App = components['schemas']['ApplicationView'];
type Kind = components['schemas']['DocumentKind'];
const statuses: Record<components['schemas']['DocumentStatus'], string> = {
  uploaded: 'Загружен',
  recognizing: 'Распознаём',
  recognized: 'Проверьте и подтвердите значения',
  failed: 'Не удалось распознать, нужен ручной ввод',
  skipped: 'Распознавание выключено, нужен ручной ввод',
  confirmed: 'Подтверждён',
};

export function Documents({ card, bank, refresh }: { card: App; bank: boolean; refresh: () => void }) {
  const reduced = useMotionDisabled();
  const [kind, setKind] = useState<Kind>(card.representative_role === 'proxy' ? 'power_of_attorney' : 'statement');
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [comment, setComment] = useState('');
  const [revenue, setRevenue] = useState('');
  const required = [
    ...new Set<Kind>([
      ...(card.info_request?.documents ?? []),
      ...(card.representative_role === 'proxy' ? ['power_of_attorney' as const] : []),
    ]),
  ];
  const choices = required.length ? required : (Object.keys(kinds) as Kind[]).filter((k) => k !== 'loan_agreement');
  const editable = !bank && ['draft', 'need_info'].includes(card.status);
  async function sample() {
    setBusy(true);
    setError('');
    try {
      const { data, error } = await api.GET('/samples/{inn}', {
        params: { path: { inn: card.company.inn }, query: { kind: 'statement', format: 'c' } },
        parseAs: 'blob',
      });
      if (data) {
        setKind('statement');
        setFile(new File([data], `sample-${card.company.inn}.png`, { type: data.type }));
      } else setError(error?.title ?? 'Пример недоступен. Выберите свой файл.');
    } catch {
      setError('Не удалось получить пример. Выберите свой файл.');
    } finally {
      setBusy(false);
    }
  }
  async function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!file || busy) return;
    if (
      !['application/pdf', 'image/png', 'image/jpeg'].includes(file.type) ||
      file.size > 10 * 1024 * 1024 ||
      file.size === 0
    ) {
      setError('Выберите PDF, PNG или JPEG размером до 10 МБ.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      const { data, error } = await api.POST('/applications/{id}/documents', {
        params: { path: { id: card.id } },
        body: { kind, file: file.name },
        bodySerializer() {
          const form = new FormData();
          form.append('kind', choices.includes(kind) ? kind : choices[0]);
          form.append('file', file);
          return form;
        },
      });
      if (data) {
        setFile(null);
        notify('Документ загружен.');
        refresh();
      } else setError(error?.title ?? 'Не удалось загрузить документ.');
    } catch {
      setError('Нет связи с сервером. Проверьте список документов перед повторной загрузкой.');
    } finally {
      setBusy(false);
    }
  }
  async function provide(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError('');
    try {
      const { data, error } = await api.POST('/applications/{id}/provide-info', {
        params: { path: { id: card.id } },
        body: {
          comment,
          document_ids: card.documents
            .filter((doc) => doc.status === 'confirmed' || !['statement', 'financials'].includes(doc.kind))
            .map((doc) => doc.id),
          ...(revenue ? { monthly_revenue_kop: kop(revenue) } : {}),
        },
      });
      if (data) {
        notify('Сведения отправлены банку.');
        refresh();
      } else setError(error?.title ?? 'Не удалось отправить сведения.');
    } catch {
      setError('Нет связи с сервером. Попробуйте ещё раз.');
    } finally {
      setBusy(false);
    }
  }
  const due = card.info_request?.due_at ?? card.due_at;
  if (!editable && !bank && !card.documents.length) return null;
  return (
    <section>
      <h2>Документы для заявки</h2>
      {editable && (
        <div className="document-checklist">
          <h3>{required.length ? 'Сейчас нужно от вас' : 'Обязательные документы пока не запрошены'}</h3>
          {!required.length && editable && (
            <p>
              Банк сообщит в этой карточке, если понадобятся документы. Дополнительный файл можно приложить по своей
              инициативе.
            </p>
          )}
          {(required.length > 1 ? required : []).map((type) => {
            const doc = card.documents.find((d) => d.kind === type);
            return (
              <div className="document-task" key={type}>
                <div>
                  <strong>{kinds[type]}</strong>
                  <p>{doc ? statuses[doc.status] : 'Нужно загрузить'}</p>
                </div>
                {editable && (
                  <Button
                    size={40}
                    onClick={() => {
                      setKind(type);
                      setFile(null);
                      document
                        .getElementById('document-upload')
                        ?.scrollIntoView({ behavior: reduced ? 'instant' : 'smooth' });
                    }}
                  >
                    Прикрепить{' '}
                    {type === 'power_of_attorney' ? 'доверенность' : type === 'statement' ? 'выписку' : 'документ'}
                  </Button>
                )}
              </div>
            );
          })}
        </div>
      )}
      {!card.documents.length && !required.length && <p>Документы ещё не загружены.</p>}
      <motion.ul layout={reduced ? false : 'position'}>
        <AnimatePresence initial={false}>
          {card.documents.map((doc) => (
            <ListItem key={doc.id}>
              <a href={`/api/documents/${encodeURIComponent(doc.id)}/file`} target="_blank" rel="noreferrer">
                {doc.file_name}
              </a>
              : {kinds[doc.kind]}, <StatusText value={doc.status}>{statuses[doc.status]}</StatusText>
            </ListItem>
          ))}
        </AnimatePresence>
      </motion.ul>
      {card.documents
        .filter((doc) => ['statement', 'financials'].includes(doc.kind))
        .map((doc) => (
          <DocumentDetails
            key={doc.id}
            id={doc.id}
            status={doc.status}
            inn={card.company.inn}
            editable={editable}
            bank={bank}
            refresh={refresh}
          />
        ))}
      {error && <BottomNotice message={error} onClose={() => setError('')} />}
      {editable && choices.includes('statement') && (
        <Button loading={busy} onClick={sample}>
          Взять пример выписки
        </Button>
      )}
      {editable && (
        <form id="document-upload" onSubmit={upload}>
          {choices.length === 1 ? (
            <h3>{kinds[choices[0]]}</h3>
          ) : (
            <Select
              block
              label="Вид документа"
              selected={choices.includes(kind) ? kind : choices[0]}
              options={choices.map((key) => ({ key, content: kinds[key] }))}
              disabled={busy}
              onChange={({ selected }) => setKind(selected!.key as Kind)}
            />
          )}
          <Attach
            aria-label="Файл документа"
            buttonContent="Выбрать файл"
            accept="application/pdf,image/png,image/jpeg"
            value={file ? [file] : null}
            onChange={(_, payload) => setFile(payload.files[0] ?? null)}
            onClear={() => setFile(null)}
            disabled={busy}
          />
          <p>PDF, PNG или JPEG · до 10 МБ · до 5 страниц.</p>
          <Button type="submit" loading={busy} disabled={!file}>
            Загрузить
          </Button>
        </form>
      )}
      {/* info_request бэк пока не отдаёт, что нужно, видно из причин выше; без формы ответить нечем и заявка висит до expired */}
      {!bank && card.status === 'need_info' && (
        <form onSubmit={provide}>
          <h2>Нужно от вас</h2>
          {card.info_request?.comment && <p>{card.info_request.comment}</p>}
          {due && <p>Ответить до: {new Date(due).toLocaleString('ru-RU')}</p>}
          {card.info_request && (
            <ul>
              {card.info_request.documents.map((doc) => (
                <li key={doc}>{kinds[doc]}</li>
              ))}
            </ul>
          )}
          {card.info_request?.reasons.map((reason, index) => (
            <p key={index}>{reason.text}</p>
          ))}
          <Input
            block
            label="Уточнённая выручка за месяц, ₽"
            type="number"
            min={1}
            step="0.01"
            value={revenue}
            onChange={(event) => setRevenue(event.target.value)}
            disabled={busy}
          />
          <Input
            block
            label="Комментарий банку"
            maxLength={1000}
            value={comment}
            onChange={(event) => setComment(event.target.value)}
            disabled={busy}
          />
          <p>Выписку и отчётность нужно подтвердить перед отправкой.</p>
          <Button type="submit" loading={busy} disabled={!card.allowed_next.includes('checking')}>
            Отправить сведения банку
          </Button>
        </form>
      )}
    </section>
  );
}
