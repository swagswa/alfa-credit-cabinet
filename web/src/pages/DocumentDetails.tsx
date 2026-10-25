import { Table } from '@alfalab/core-components/table';
import { Typography } from '@alfalab/core-components/typography';
import { InlineNotice } from '../ui/InlineNotice';
import { useEffect, useState } from 'react';
import { Button } from '../motion';
import { api } from '../api/client';
import { fieldNames, money } from '../api/presentation';
import { FinancialsForm } from './FinancialsForm';
import { StatementForm } from './StatementForm';
import type { components } from '../api/schema.gen';
type Doc = components['schemas']['Document'];
// в api копейки, банку показываем рубли
const show = (key: string, value: unknown) =>
  value === undefined
    ? 'Нет'
    : key.endsWith('_kop')
      ? money(value as number)
      : key === 'monthly_in'
        ? (value as { month: string; amount_kop: number }[]).map((m) => `${m.month}: ${money(m.amount_kop)}`).join(', ')
        : String(value);
export function DocumentDetails({
  id,
  inn,
  editable,
  bank,
  refresh,
  status,
}: {
  id: string;
  inn: string;
  editable: boolean;
  bank: boolean;
  refresh: () => void;
  status: Doc['status'];
}) {
  const [document, setDocument] = useState<Doc>();
  const [error, setError] = useState('');
  const [manual, setManual] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    async function load() {
      try {
        const { data } = await api.GET('/documents/{id}', { params: { path: { id } } });
        if (!active) return;
        if (data) {
          setDocument(data);
          setError('');
          if (['recognizing', 'uploaded'].includes(data.status)) timer = setTimeout(load, 2000);
        } else setError('Не удалось загрузить данные документа.');
      } catch {
        if (active) setError('Нет связи с сервером документов.');
      }
    }
    void load();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [id, revision, status]);
  const initial = document?.confirmed ?? document?.extracted;
  const problems = Object.fromEntries((document?.validation_errors ?? []).map((e) => [e.field, e.message]));
  return (
    <div className="document-details">
      {document && (
        <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
          {document.file_name}
        </Typography.Title>
      )}
      {error && <InlineNotice>{error}</InlineNotice>}
      {error && (
        <Button size={40} onClick={() => setRevision((v) => v + 1)}>
          Повторить загрузку данных
        </Button>
      )}
      {document && ['recognizing', 'uploaded'].includes(document.status) && (
        <div>
          <Typography.Text tag="p" view="primary-small" role="status">
            Распознаём документ - обычно это занимает около минуты.
          </Typography.Text>
          {editable && !manual && (
            <Button size={40} onClick={() => setManual(true)}>
              Заполнить вручную
            </Button>
          )}
        </div>
      )}
      {document?.error && (
        <InlineNotice>{document.error[0].toUpperCase() + document.error.slice(1)}. Доступен ручной ввод.</InlineNotice>
      )}
      {problems.inn && <InlineNotice>ИНН: {problems.inn}</InlineNotice>}
      {editable &&
        document &&
        document.status !== 'confirmed' &&
        (manual || !['recognizing', 'uploaded'].includes(document.status)) && (
          <div className={initial && !manual ? 'extracted-fields' : ''} key={id}>
            {initial && !manual && (
              <Typography.Text tag="p" view="primary-small">
                Данные из файла подставлены в поля. Проверьте их перед подтверждением.
              </Typography.Text>
            )}
            {document?.kind === 'financials' && (
              <FinancialsForm
                documentId={id}
                inn={inn}
                initial={initial?.financials}
                problems={problems}
                refresh={() => {
                  setRevision((v) => v + 1);
                  refresh();
                }}
              />
            )}
            {document?.kind === 'statement' && (
              <StatementForm
                documentId={id}
                inn={inn}
                initial={initial?.statement}
                problems={problems}
                refresh={() => {
                  setRevision((v) => v + 1);
                  refresh();
                }}
              />
            )}
          </div>
        )}
      {bank && document && (
        <>
          <Typography.Text tag="p" view="primary-small">
            Модель: {document.model ?? 'Ручной ввод'}. Подтверждено:{' '}
            {document.confirmed_at ? new Date(document.confirmed_at).toLocaleString('ru-RU') : 'Нет'}.
          </Typography.Text>
          <div className="table-scroll">
            <Table>
              <caption>Данные модели и клиента</caption>
              <Table.THead>
                <Table.THeadCell>Поле</Table.THeadCell>
                <Table.THeadCell>Модель</Table.THeadCell>
                <Table.THeadCell>Клиент</Table.THeadCell>
              </Table.THead>
              <Table.TBody>
                {Object.entries(
                  document.confirmed?.statement ??
                    document.confirmed?.financials ??
                    document.extracted?.statement ??
                    document.extracted?.financials ??
                    {},
                ).map(([key, value]) => {
                  const raw = document.extracted?.statement ?? document.extracted?.financials;
                  return (
                    <Table.TRow key={key} className={document.edited_fields?.includes(key) ? 'extracted' : ''}>
                      <Table.TCell>
                        {fieldNames[key] ?? key}
                        {document.edited_fields?.includes(key) ? ' (изменено)' : ''}
                      </Table.TCell>
                      <Table.TCell>{show(key, raw?.[key as keyof typeof raw])}</Table.TCell>
                      <Table.TCell>{show(key, value)}</Table.TCell>
                    </Table.TRow>
                  );
                })}
              </Table.TBody>
            </Table>
          </div>
        </>
      )}
    </div>
  );
}
