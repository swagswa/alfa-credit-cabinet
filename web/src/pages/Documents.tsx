import { DocumentUpload } from './DocumentUpload';
import { Disclosure } from '../ui/Disclosure';
import { MoneyField } from '../ui/MoneyField';
import { Typography } from '@alfalab/core-components/typography';
import { motion, AnimatePresence, Button, ListItem, Select, StatusText, useMotionDisabled } from '../motion';
import { BottomNotice, notify } from '../BottomNotice';
import { useState, type FormEvent } from 'react';
import { Input } from '@alfalab/core-components/input';
import { Attach } from '@alfalab/core-components/attach';
import { api, errorText } from '../api/client';
import { kinds, kop, requiredDocs } from '../api/presentation';
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

export function Documents({
  card,
  bank,
  product,
  refresh,
}: {
  card: App;
  bank: boolean;
  product?: components['schemas']['Product'];
  refresh: () => void;
}) {
  const reduced = useMotionDisabled();
  const [kind, setKind] = useState<Kind>(card.representative_role === 'proxy' ? 'power_of_attorney' : 'passport');
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [comment, setComment] = useState('');
  const [revenue, setRevenue] = useState('');
  const required = [
    ...new Set<Kind>([
      ...(card.info_request?.documents ?? []),
      ...(card.status === 'draft' && card.representative_role === 'proxy' ? ['power_of_attorney' as const] : []),
    ]),
  ];
  const editable = !bank && ['draft', 'need_info'].includes(card.status);
  const draft = card.status === 'draft';
  const checklist = [...new Set(draft ? requiredDocs(card, product) : required)];
  const choices = draft
    ? (Object.keys(kinds) as Kind[]).filter((k) => k !== 'loan_agreement' && !checklist.includes(k))
    : required;
  const financialRequested = required.some((type) => ['statement', 'financials'].includes(type));
  const revenueRequested = [...card.reasons, ...(card.info_request?.reasons ?? [])].some(
    (reason) => reason.code === 'SF_REVENUE_MISMATCH',
  );
  const missingRequired = required.some(
    (type) =>
      !card.documents.some(
        (doc) => doc.kind === type && (!['statement', 'financials'].includes(type) || doc.status === 'confirmed'),
      ),
  );
  async function sample() {
    setBusy(true);
    setError('');
    try {
      // текстовый PDF: скан формата c у кофейни OCR читает с ошибкой в одной цифре, и показ ИИ проваливался
      const { data, error } = await api.GET('/samples/{inn}', {
        params: { path: { inn: card.company.inn }, query: { kind: 'statement', format: 'a' } },
        parseAs: 'blob',
      });
      setBusy(false);
      if (data) {
        setKind('statement');
        await send(new File([data], `sample-${card.company.inn}.pdf`, { type: data.type }), 'statement');
      } else setError(errorText(error, 'Пример недоступен. Выберите свой файл.'));
    } catch {
      setError('Не удалось получить пример. Выберите свой файл.');
      setBusy(false);
    }
  }
  function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (file) void send(file, choices.includes(kind) ? kind : choices[0]);
  }
  async function send(file: File, kind: Kind) {
    if (busy) return false;
    if (
      !['application/pdf', 'image/png', 'image/jpeg'].includes(file.type) ||
      file.size > 10 * 1024 * 1024 ||
      file.size === 0
    ) {
      setError('Выберите PDF, PNG или JPEG размером до 10 МБ.');
      return false;
    }
    setBusy(true);
    setError('');
    try {
      const { data, error } = await api.POST('/applications/{id}/documents', {
        params: { path: { id: card.id } },
        body: { kind, file: file.name },
        bodySerializer() {
          const form = new FormData();
          form.append('kind', kind);
          form.append('file', file);
          return form;
        },
      });
      if (data) {
        setFile(null);
        notify('Документ загружен.');
        refresh();
        return true;
      } else setError(errorText(error, 'Не удалось загрузить документ.'));
    } catch {
      setError('Нет связи с сервером. Проверьте список документов перед повторной загрузкой.');
    } finally {
      setBusy(false);
    }
    return false;
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
      } else setError(errorText(error, 'Не удалось отправить сведения.'));
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
      <Typography.Title tag="h2" view="small" font="system" defaultMargins>
        {card.status === 'need_info' && !bank ? 'Нужно от вас' : 'Документы для заявки'}
      </Typography.Title>
      {editable && !draft && (
        <>
          {[
            ...new Map(
              [...card.reasons, ...(card.info_request?.reasons ?? [])].map((reason) => [reason.code, reason]),
            ).values(),
          ].map((reason) => (
            <Typography.Text tag="p" view="primary-small" key={reason.code}>
              {reason.text}
            </Typography.Text>
          ))}
          {card.info_request?.comment && (
            <Typography.Text tag="p" view="primary-small">
              Комментарий банка: {card.info_request.comment}
            </Typography.Text>
          )}
          {due && (
            <Typography.Text tag="p" view="primary-small">
              Ответить до: {new Date(due).toLocaleString('ru-RU')}
            </Typography.Text>
          )}
        </>
      )}
      {editable && (
        <div className="document-checklist">
          {draft && (
            <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
              {draft && checklist.length
                ? 'Что приложить до отправки'
                : required.length || card.status === 'need_info'
                  ? 'Сейчас нужно от вас'
                  : 'Обязательные документы пока не запрошены'}
            </Typography.Title>
          )}
          {draft && !!checklist.length && (
            <Typography.Text tag="p" view="primary-small">
              Без этих документов банк запросит их уже после отправки, и на ответ будет ограниченное время.
              {product?.documents?.some((d) => d.when === 'no_turnover') &&
                ' Выписку из другого банка могут попросить, если оборотов по вашему счёту у нас мало.'}
            </Typography.Text>
          )}
          {!required.length && !(draft && checklist.length) && editable && (
            <Typography.Text tag="p" view="primary-small">
              {card.status === 'need_info'
                ? 'Оставьте пояснение для банка в поле ниже.'
                : 'Банк сообщит в этой карточке, если понадобятся документы.'}
              {draft && ' Дополнительный файл можно приложить по своей инициативе.'}
            </Typography.Text>
          )}
          {checklist.map((type) => {
            const doc = card.documents.find((d) => d.kind === type);
            return (
              <div className="document-task document-upload-task" role="group" aria-label={kinds[type]} key={type}>
                <div>
                  <strong>{kinds[type]}</strong>
                  <Typography.Text tag="p" view="primary-small">
                    {doc ? statuses[doc.status] : 'Нужно загрузить'}
                  </Typography.Text>
                </div>
                <DocumentUpload kind={type} busy={busy} uploaded={!!doc} onUpload={send} />
              </div>
            );
          })}
        </div>
      )}
      {!card.documents.length && !required.length && (
        <Typography.Text tag="p" view="primary-small">
          Документы ещё не загружены.
        </Typography.Text>
      )}
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
      {editable && (draft || financialRequested) && choices.includes('statement') && (
        <Button loading={busy} onClick={sample}>
          Загрузить пример выписки
        </Button>
      )}
      {editable && draft && choices.length > 0 && (
        <Disclosure label="Загрузить дополнительный документ">
          <form id="document-upload" onSubmit={upload}>
            <Typography.Text tag="p" view="primary-small">
              Загружается: <strong>{kinds[choices.includes(kind) ? kind : choices[0]]}</strong>
            </Typography.Text>
            {choices.length > 1 && (
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
            <Typography.Text tag="p" view="primary-small">
              PDF, PNG или JPEG · до 10 МБ · до 5 страниц.
            </Typography.Text>
            <Button type="submit" loading={busy} disabled={!file}>
              Загрузить
            </Button>
          </form>
        </Disclosure>
      )}
      {!bank && card.status === 'need_info' && (
        <form onSubmit={provide}>
          {revenueRequested && (
            <MoneyField label="Уточнённая выручка за месяц, ₽" value={revenue} onChange={setRevenue} disabled={busy} />
          )}
          <Input
            block
            label="Комментарий банку"
            maxLength={1000}
            value={comment}
            onChange={(event) => setComment(event.target.value)}
            disabled={busy}
          />
          {financialRequested && (
            <Typography.Text tag="p" view="primary-small">
              Проверьте и подтвердите распознанные значения перед отправкой.
            </Typography.Text>
          )}
          <Button type="submit" loading={busy} disabled={missingRequired || !card.allowed_next.includes('checking')}>
            Отправить сведения банку
          </Button>
        </form>
      )}
    </section>
  );
}
