import { useEffect, useState, type ReactNode } from 'react';
import { api, settle, useHistory } from '../api/client';
import { fieldNames, goals, kinds, labels, money, rate, products, representatives } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { Progress, steps } from './Progress';
import { Decision } from './Decision';
import { MotionDetails } from '../motion';

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
    <MotionDetails className="saved-document-values">
      <summary>Посмотреть подтверждённые данные</summary>
      {failed ? (
        <p role="alert">Не удалось загрузить данные документа.</p>
      ) : !document ? (
        <p role="status">Загружаем данные…</p>
      ) : !values ? (
        <p>Подтверждённые значения не сохранены.</p>
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
    </MotionDetails>
  );
}

export function CompletedJourney({ card }: { card: App }) {
  const [step, setStep] = useState(4);
  const { history, state: historyState } = useHistory(card.id, card.version);
  const issued = ['issued', 'closed'].includes(card.status);
  return (
    <>
      <Progress card={card} selected={step} onSelect={setStep} />
      <h2>Заявка № {card.number}</h2>
      <div id="journey-view" className="journey-view" role="region" aria-label={steps[step]}>
        {step !== 4 && <p className="archive-caption">Заявка завершена · Только просмотр</p>}
        {step === 0 && (
          <section className="archive-panel">
            <h3>Данные заявки</h3>
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
            <p className="hint">Сохранённые данные заявки. Предыдущие версии полей не передаются сервером.</p>
          </section>
        )}
        {step === 1 && (
          <section className="archive-panel">
            <h3>Рассмотрение заявки</h3>
            {card.submitted_at ? (
              <p>Отправлена в банк: {date(card.submitted_at)}</p>
            ) : (
              <p>Дата отправки не сохранена.</p>
            )}
            {historyState === 'loading' && <p role="status">Загружаем историю…</p>}
            {historyState === 'error' && <p role="alert">История временно недоступна.</p>}
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
                <p>История этапов не сохранена.</p>
              ))}
          </section>
        )}
        {step === 2 && (
          <section className="archive-panel">
            <h3>Предложение банка</h3>
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
                {issued && <p>Условия приняты, финансирование оформлено.</p>}
              </>
            ) : (
              <p>По этой заявке предложение банка не сформировано.</p>
            )}
          </section>
        )}
        {step === 3 && (
          <section className="archive-panel">
            <h3>Документы и подпись</h3>
            {card.documents.length ? (
              <ul className="archive-documents">
                {card.documents.map((document) => (
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
              <p>Прикреплённых документов нет.</p>
            )}
            {issued ? <p className="archive-signature">Документы подписаны.</p> : <p>Подписание не завершено.</p>}
          </section>
        )}
        {step === 4 && <Decision card={{ ...card, allowed_next: [] }} refresh={() => undefined} />}
      </div>
    </>
  );
}
