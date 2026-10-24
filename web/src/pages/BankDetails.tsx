import { useState } from 'react';
import { Button, MotionDetails } from '../motion';
import { BankActions } from './BankActions';
import { useHistory } from '../api/client';
import { isCompleted } from '../api/dashboard';
import { actors, labels, money, rate, products, sources } from '../api/presentation';
import type { components } from '../api/schema.gen';
export function BankDetails({
  card,
  role,
}: {
  card: components['schemas']['ApplicationView'];
  role?: components['schemas']['Role'];
}) {
  const [revision, setRevision] = useState(0);
  const { history, state } = useHistory(card.id, card.version, revision);
  const finished = isCompleted(card.status);
  const lastDecision = [...history].reverse().find((event) => event.to_status === card.status);
  return (
    <section>
      {finished && (
        <div className={card.status === 'rejected' ? 'rejection-result' : 'bank-result'}>
          <p className="eyebrow">Итог рассмотрения</p>
          <h2>{labels[card.status]}</h2>
          <p>
            {card.status === 'rejected'
              ? 'Заявка завершена отказом. Повторное решение и запрос документов недоступны.'
              : 'Работа по текущему этапу завершена. Ниже доступны материалы и история заявки.'}
          </p>
          {card.reasons.map((reason, index) => (
            <p key={`${reason.code}-${index}`}>
              <strong>{reason.code}</strong>: {reason.text}
            </p>
          ))}
          {lastDecision && (
            <p className="hint">
              {lastDecision.actor_name ?? actors[lastDecision.actor_role]} ·{' '}
              {new Date(lastDecision.created_at).toLocaleString('ru-RU')}
              {lastDecision.comment && <> · {lastDecision.comment}</>}
            </p>
          )}
          <a href="#bank-audit" className="back-link">
            Посмотреть историю решения
          </a>
        </div>
      )}
      {!!card.allowed_next.length && (
        <BankActions
          key={card.id + card.status}
          card={card}
          role={role}
          refresh={() => {
            setRevision((v) => v + 1);
            window.dispatchEvent(new Event('application-refresh'));
          }}
        />
      )}
      <h2>Проверки источников</h2>
      {!card.checks?.length && <p>Результаты проверок ещё не поступили.</p>}
      {card.checks?.map((check, index) => (
        <article key={`${check.source}-${index}`}>
          <h3>{sources[check.source]}</h3>
          <p>
            Результат:{' '}
            {(
              { ok: 'Успешно', unavailable: 'Источник недоступен', error: 'Ошибка', not_found: 'Не найдено' } as Record<
                string,
                string
              >
            )[check.status] ?? check.status}
            . Проверено: {new Date(check.checked_at).toLocaleString('ru-RU')}.
          </p>
          {check.error && <p role="alert">{check.error}</p>}
          {check.payload && (
            <MotionDetails>
              <summary>Ответ источника</summary>
              <pre>{JSON.stringify(check.payload, null, 2)}</pre>
            </MotionDetails>
          )}
        </article>
      ))}
      {/* у автоматического решения рекомендации нет, пустой блок только сбивает */}
      {(card.recommendation || card.status === 'manual_review') && (
        <>
          <h2>Рекомендация политики</h2>
          {card.recommendation ? (
            <p>
              {products[card.recommendation.product]}: {money(card.recommendation.amount_kop)},{' '}
              {card.recommendation.term_months} мес., {rate(card.recommendation.rate_bp)} годовых.{' '}
              {card.recommendation.guarantor_required ? 'Нужно поручительство.' : ''}{' '}
              {card.recommendation.pledge_required ? 'Нужен залог.' : ''}
            </p>
          ) : (
            <p>Рекомендация пока не поступила.</p>
          )}
        </>
      )}
      <h2>Оценка риска</h2>
      <p className="hint">
        Скоринг: оценка риска по набору факторов. Решение банка также учитывает проверки и условия продукта.
        Стоп-фактор: обстоятельство, требующее отказа или ручной проверки.
      </p>
      {card.scoring ? (
        <>
          <p>
            Баллы: {card.scoring.score} из 100. Класс риска: {card.scoring.grade}. Вероятность невозврата:{' '}
            {card.scoring.pd}.
          </p>
          <p>
            Версия модели: {card.scoring.version}. Версия политики: {card.policy_version ?? 'не указана'}.
          </p>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Фактор</th>
                  <th>Значение</th>
                  <th>Баллы</th>
                  <th>Код причины</th>
                </tr>
              </thead>
              <tbody>
                {card.scoring.factors.map((factor, index) => (
                  <tr key={`${factor.code}-${index}`}>
                    <th>{factor.name}</th>
                    <td>{factor.value.toLocaleString('ru-RU', { maximumFractionDigits: 2 })}</td>
                    <td>
                      {factor.points} / {factor.max_points}
                    </td>
                    <td>{factor.reason ?? 'Нет'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      ) : (
        <p>Оценка риска ещё не рассчитана. При отказе по стоп-фактору расчёт может не выполняться.</p>
      )}
      {!!card.reasons.length && (
        <>
          <h3>Причины решения и стоп-факторы</h3>
          <ul>
            {card.reasons.map((reason, index) => (
              <li key={`${reason.code}-${index}`}>
                {reason.code}: {reason.text}
              </li>
            ))}
          </ul>
        </>
      )}
      <h2 id="bank-audit">История заявки</h2>
      {state === 'error' && <p role="alert">Не удалось загрузить историю.</p>}
      <Button size={40} onClick={() => setRevision((value) => value + 1)}>
        Обновить историю
      </Button>
      {state === 'ready' && !history.length && <p>Событий пока нет.</p>}
      <ol>
        {history.map((event) => (
          <li key={event.id}>
            <time>{new Date(event.created_at).toLocaleString('ru-RU')}</time>:{' '}
            {event.actor_name ?? actors[event.actor_role]},{' '}
            {
              {
                transition: 'Смена этапа',
                override: 'Отклонение от рекомендации',
                escalation: 'Превышен срок рассмотрения',
                signing: 'Подписание',
              }[event.kind]
            }
            {event.from_status && <>: {labels[event.from_status]}</>}
            {event.to_status && <> → {labels[event.to_status]}</>}
            {event.comment && <p>{event.comment}</p>}
            {!!event.reason_codes?.length && <p>Причины: {event.reason_codes.join(', ')}</p>}
          </li>
        ))}
      </ol>
    </section>
  );
}
