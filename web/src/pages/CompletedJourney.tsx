import { Typography } from '@alfalab/core-components/typography';
import { InlineNotice } from '../ui/InlineNotice';
import { Disclosure } from '../ui/Disclosure';
import { useEffect, useState, type ReactNode } from 'react';
import { api, settle, useHistory } from '../api/client';
import { fieldNames, goals, kinds, labels, money, rate, products, representatives } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { Progress, steps } from './Progress';
import { Decision } from './Decision';
import { StatusBadge } from '../ui/Workspace';

type App = components['schemas']['ApplicationView'];
const date = (value: string) => new Date(value).toLocaleString('ru-RU', { dateStyle: 'medium', timeStyle: 'short' });
function Field({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div>
      <dt>{title}</dt>
      <dd>{children}</dd>
    </div>
  );
}

function SavedDocumentValues({ id }: { id: string }) {
  const [document, setDocument] = useState<components['schemas']['Document']>();
  const [failed, setFailed] = useState(false);
  useEffect(
    () =>
      settle(
        api.GET('/documents/{id}', { params: { path: { id } } }),
        ({ data }) => {
          if (data) setDocument(data);
          else setFailed(true);
        },
        () => setFailed(true),
      ),
    [id],
  );
  const values = document?.confirmed?.statement ?? document?.confirmed?.financials;
  return (
    <Disclosure className="saved-document-values" label={<>Посмотреть подтверждённые данные</>}>
      {failed ? (
        <InlineNotice>Не удалось загрузить данные документа.</InlineNotice>
      ) : !document ? (
        <Typography.Text tag="p" view="primary-small" role="status">
          Загружаем данные…
        </Typography.Text>
      ) : !values ? (
        <Typography.Text tag="p" view="primary-small">
          Подтверждённые значения не сохранены.
        </Typography.Text>
      ) : (
        <dl className="archive-fields">
          {Object.entries(values)
            // поступления по месяцам выводятся ниже, по полю на месяц
            .filter(([key]) => key in fieldNames && key !== 'monthly_in')
            .map(([key, value]) => (
              <Field key={key} title={fieldNames[key]}>
                {key.endsWith('_kop') && typeof value === 'number' ? money(value) : String(value)}
              </Field>
            ))}
          {document.confirmed?.statement?.monthly_in.map((month) => (
            <Field key={month.month} title={`Поступления за ${month.month}`}>
              {money(month.amount_kop)}
            </Field>
          ))}
        </dl>
      )}
    </Disclosure>
  );
}

function SavedDocuments({ documents }: { documents: App['documents'] }) {
  return (
    <>
      {documents.length ? (
        <ul className="archive-documents">
          {documents.map((document) => (
            <li key={document.id}>
              <span>{kinds[document.kind]}</span>
              <a href={`/api/documents/${encodeURIComponent(document.id)}/file`} target="_blank" rel="noreferrer">
                {document.file_name}
              </a>
              <small>{document.status === 'confirmed' ? 'Данные подтверждены' : 'Сохранённый документ'}</small>
              {['statement', 'financials'].includes(document.kind) && <SavedDocumentValues id={document.id} />}
            </li>
          ))}
        </ul>
      ) : (
        <Typography.Text tag="p" view="primary-small">
          Прикреплённых документов нет.
        </Typography.Text>
      )}
    </>
  );
}

export function CompletedJourney({ card }: { card: App }) {
  const [step, setStep] = useState(4);
  const { history, state: historyState } = useHistory(card.id, card.version);
  const issued = ['issued', 'closed'].includes(card.status);
  return (
    <>
      <Progress card={card} selected={step} onSelect={setStep} />
      <Typography.Title tag="h2" view="small" font="system" defaultMargins>
        Заявка № {card.number}
      </Typography.Title>
      {step === 4 && ['expired', 'cancelled_by_client'].includes(card.status) && <StatusBadge status={card.status} />}
      <div id="journey-view" className="journey-view" role="region" aria-label={steps[step]}>
        {step !== 4 && (
          <Typography.Text tag="p" view="primary-small" className="archive-caption">
            Заявка завершена · Только просмотр
          </Typography.Text>
        )}
        {step === 0 && (
          <section className="archive-panel">
            <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
              Данные заявки
            </Typography.Title>
            <dl className="archive-fields">
              <Field title="Компания">{card.company.short_name}</Field>
              <Field title="ИНН">{card.company.inn}</Field>
              <Field title="Цель">{goals[card.purpose]}</Field>
              <Field title="Продукт">{products[card.product]}</Field>
              <Field title="Запрошенная сумма">{money(card.amount_kop)}</Field>
              <Field title="Запрошенный срок">{card.term_months} мес.</Field>
              <Field title="Представитель">{representatives[card.representative_role]}</Field>
              <Field title="Согласие на запрос кредитной истории">
                {card.bki_consent ? 'Предоставлено' : 'Не предоставлено'}
              </Field>
              {card.monthly_revenue_kop !== undefined && (
                <Field title="Среднемесячная выручка">{money(card.monthly_revenue_kop)}</Field>
              )}
              {card.equipment_cost_kop !== undefined && (
                <Field title="Стоимость оборудования">{money(card.equipment_cost_kop)}</Field>
              )}
              {card.comment && <Field title="Комментарий">{card.comment}</Field>}
            </dl>
            <Typography.Text tag="p" view="primary-small" className="hint">
              Сохранённые данные заявки. Предыдущие версии полей не передаются сервером.
            </Typography.Text>
          </section>
        )}
        {step === 1 && (
          <section className="archive-panel">
            <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
              Рассмотрение заявки
            </Typography.Title>
            {card.submitted_at ? (
              <Typography.Text tag="p" view="primary-small">
                Отправлена в банк: {date(card.submitted_at)}
              </Typography.Text>
            ) : (
              <Typography.Text tag="p" view="primary-small">
                Дата отправки не сохранена.
              </Typography.Text>
            )}
            {historyState === 'loading' && (
              <Typography.Text tag="p" view="primary-small" role="status">
                Загружаем историю…
              </Typography.Text>
            )}
            {historyState === 'error' && <InlineNotice>История временно недоступна.</InlineNotice>}
            {historyState === 'ready' &&
              (history.length ? (
                <ol className="archive-timeline">
                  {history
                    .filter((event) => event.kind === 'transition')
                    .map((event) => (
                      <li key={event.id}>
                        <time dateTime={event.created_at}>{date(event.created_at)}</time>
                        <strong>{event.to_status ? labels[event.to_status] : 'Обновление заявки'}</strong>
                      </li>
                    ))}
                </ol>
              ) : (
                <Typography.Text tag="p" view="primary-small">
                  История этапов не сохранена.
                </Typography.Text>
              ))}
          </section>
        )}
        {step === 2 && (
          <section className={`archive-panel${card.status === 'expired' ? ' expired-offer' : ''}`}>
            <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
              Предложение банка
            </Typography.Title>
            {card.status === 'expired' && card.offer && (
              <Typography.Text tag="p" view="primary-small">
                Сохранённые условия. Предложение больше не действует.
              </Typography.Text>
            )}
            {card.offer ? (
              <>
                <dl className="archive-fields">
                  <Field title="Продукт">{products[card.offer.product]}</Field>
                  <Field title="Сумма">{money(card.offer.amount_kop)}</Field>
                  <Field title="Срок">{card.offer.term_months} мес.</Field>
                  <Field title="Ставка">{rate(card.offer.rate_bp)} годовых</Field>
                  <Field title="Ежемесячный платёж">{money(card.offer.monthly_payment_kop)}</Field>
                  <Field title="Залог">{card.offer.pledge_required ? 'Требуется' : 'Без залога'}</Field>
                  <Field title="Поручительство собственника">
                    {card.offer.guarantor_required ? 'Требуется' : 'Не требуется'}
                  </Field>
                  {card.offer.msp_guarantee && <Field title="Поручительство МСП">Предусмотрено</Field>}
                </dl>
                {issued && (
                  <Typography.Text tag="p" view="primary-small">
                    Условия приняты, финансирование оформлено.
                  </Typography.Text>
                )}
              </>
            ) : (
              <Typography.Text tag="p" view="primary-small">
                По этой заявке предложение банка не сформировано.
              </Typography.Text>
            )}
          </section>
        )}
        {step === 3 && (
          <section className="archive-panel">
            <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
              Документы и подпись
            </Typography.Title>
            <SavedDocuments documents={card.documents} />
            {issued ? (
              <Typography.Text tag="p" view="primary-small" className="archive-signature">
                Документы подписаны.
              </Typography.Text>
            ) : (
              <Typography.Text tag="p" view="primary-small">
                Подписание не завершено.
              </Typography.Text>
            )}
          </section>
        )}
        {step === 4 && (
          <>
            <Decision card={{ ...card, allowed_next: [] }} refresh={() => undefined} />
            {issued && (
              <section className="archive-panel repayment-summary">
                <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
                  {(card.offer?.product ?? card.product) === 'overdraft' ? 'Погашение овердрафта' : 'Первый платёж'}
                </Typography.Title>
                {(card.offer?.product ?? card.product) === 'overdraft' ? (
                  <Typography.Text tag="p" view="primary-small">
                    Задолженность погашается поступлениями на расчётный счёт. Фиксированного ежемесячного платежа нет.
                  </Typography.Text>
                ) : (
                  <>
                    {card.offer && card.offer.monthly_payment_kop > 0 ? (
                      <strong className="repayment-amount">{money(card.offer.monthly_payment_kop)}</strong>
                    ) : (
                      <Typography.Text tag="p" view="primary-small">
                        Сумма платежа пока не передана банком.
                      </Typography.Text>
                    )}
                    <Typography.Text tag="p" view="primary-small" className="hint">
                      Дата первого платежа пока не передана банком.
                    </Typography.Text>
                  </>
                )}
              </section>
            )}
            {issued && (
              <section className="archive-panel" aria-label="Документы заявки">
                <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
                  Документы заявки
                </Typography.Title>
                <SavedDocuments
                  documents={[...card.documents].sort(
                    (a, b) => Number(b.kind === 'loan_agreement') - Number(a.kind === 'loan_agreement'),
                  )}
                />
              </section>
            )}
          </>
        )}
      </div>
    </>
  );
}
