import { useState } from 'react';
import { Button, MotionDetails } from '../motion';
import { BankActions } from './BankActions';
import { useHistory } from '../api/client';
import { isCompleted } from '../api/dashboard';
import { actors, bankReasons, factorValue, labels, money, rate, products, sources } from '../api/presentation';
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
  const texts = new Map(card.reasons.map((r) => [r.code, r.text]));
  const reasonText = (code: components['schemas']['ReasonCode']) => texts.get(code) || bankReasons[code] || code;
  const reasonList = card.reasons.map((reason, index) => (
    <li key={`${reason.code}-${index}`}>
      {reasonText(reason.code)} <span className="hint">{reason.code}</span>
    </li>
  ));
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
          <ul>{reasonList}</ul>
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
      {card.offer && (
        <>
          <h2>Предложено клиенту</h2>
          <p>
            {products[card.offer.product]}: {money(card.offer.amount_kop)}, {card.offer.term_months} мес.,{' '}
            {rate(card.offer.rate_bp)} годовых, платёж {money(card.offer.monthly_payment_kop)} в месяц.{' '}
            {card.offer.guarantor_required ? 'Нужно поручительство.' : ''}{' '}
            {card.offer.pledge_required ? 'Нужен залог.' : ''}
          </p>
          {(card.status === 'approved' || card.status === 'counter_offer') && (
            <p className="hint">Клиент может принять до {new Date(card.offer.expires_at).toLocaleString('ru-RU')}.</p>
          )}
        </>
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
          {check.status === 'unavailable' && (
            <p role="alert">
              Источник не ответил и после повторных попыток. Когда он заработает, нажмите «Повторить проверки».
            </p>
          )}
          {check.error && <p className="hint">Ответ системы: {check.error}</p>}
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
                  <th>Влияние</th>
                </tr>
              </thead>
              <tbody>
                {card.scoring.factors.map((factor, index) => (
                  <tr key={`${factor.code}-${index}`}>
                    <th>{factor.name}</th>
                    <td>{factorValue(factor.code, factor.value)}</td>
                    <td>
                      {factor.points} / {factor.max_points}
                    </td>
                    <td>
                      {factor.points === factor.max_points
                        ? 'лучшее значение'
                        : `минус ${factor.max_points - factor.points} от максимума`}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      ) : (
        <p>
          {card.status === 'submitted' || card.status === 'checking'
            ? 'Оценка появится после проверок источников.'
            : 'Оценку не считали: решение принято по стоп-факторам или не хватило данных источников.'}
        </p>
      )}
      {!!card.reasons.length && (
        <>
          <h3>Причины решения и стоп-факторы</h3>
          <ul>{reasonList}</ul>
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
            {!!event.reason_codes?.length && <p>Причины: {event.reason_codes.map(reasonText).join('; ')}</p>}
          </li>
        ))}
      </ol>
    </section>
  );
}
