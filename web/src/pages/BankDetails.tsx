import { Table } from '@alfalab/core-components/table';
import { Typography } from '@alfalab/core-components/typography';
import { InlineNotice } from '../ui/InlineNotice';
import { Disclosure } from '../ui/Disclosure';
import { useState } from 'react';
import { Button } from '../motion';
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
          <Typography.Text tag="p" view="primary-small" className="eyebrow">
            Итог рассмотрения
          </Typography.Text>
          <Typography.Title tag="h2" view="small" font="system" defaultMargins>
            {labels[card.status]}
          </Typography.Title>
          <Typography.Text tag="p" view="primary-small">
            {card.status === 'rejected'
              ? 'Заявка завершена отказом. Повторное решение и запрос документов недоступны.'
              : 'Работа по текущему этапу завершена. Ниже доступны материалы и история заявки.'}
          </Typography.Text>
          <ul>{reasonList}</ul>
          {lastDecision && (
            <Typography.Text tag="p" view="primary-small" className="hint">
              {lastDecision.actor_name ?? actors[lastDecision.actor_role]} ·{' '}
              {new Date(lastDecision.created_at).toLocaleString('ru-RU')}
              {lastDecision.comment && <> · {lastDecision.comment}</>}
            </Typography.Text>
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
          <Typography.Title tag="h2" view="small" font="system" defaultMargins>
            Предложено клиенту
          </Typography.Title>
          <Typography.Text tag="p" view="primary-small">
            {products[card.offer.product]}: {money(card.offer.amount_kop)}, {card.offer.term_months} мес.,{' '}
            {rate(card.offer.rate_bp)} годовых, платёж {money(card.offer.monthly_payment_kop)} в месяц.{' '}
            {card.offer.guarantor_required ? 'Нужно поручительство.' : ''}{' '}
            {card.offer.pledge_required ? 'Нужен залог.' : ''}
          </Typography.Text>
          {(card.status === 'approved' || card.status === 'counter_offer') && (
            <Typography.Text tag="p" view="primary-small" className="hint">
              Клиент может принять до {new Date(card.offer.expires_at).toLocaleString('ru-RU')}.
            </Typography.Text>
          )}
        </>
      )}
      <Typography.Title tag="h2" view="small" font="system" defaultMargins>
        Проверки источников
      </Typography.Title>
      {!card.checks?.length && (
        <Typography.Text tag="p" view="primary-small">
          Результаты проверок ещё не поступили.
        </Typography.Text>
      )}
      {card.checks?.map((check, index) => (
        <article key={`${check.source}-${index}`}>
          <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
            {sources[check.source]}
          </Typography.Title>
          <Typography.Text tag="p" view="primary-small">
            Результат:{' '}
            {(
              { ok: 'Успешно', unavailable: 'Источник недоступен', error: 'Ошибка', not_found: 'Не найдено' } as Record<
                string,
                string
              >
            )[check.status] ?? check.status}
            . Проверено: {new Date(check.checked_at).toLocaleString('ru-RU')}.
          </Typography.Text>
          {check.status === 'unavailable' && (
            <InlineNotice>
              Источник не ответил и после повторных попыток. Когда он заработает, нажмите «Повторить проверки».
            </InlineNotice>
          )}
          {check.error && (
            <Typography.Text tag="p" view="primary-small" className="hint">
              Ответ системы: {check.error}
            </Typography.Text>
          )}
          {check.payload && (
            <Disclosure label={<>Ответ источника</>}>
              <pre>{JSON.stringify(check.payload, null, 2)}</pre>
            </Disclosure>
          )}
        </article>
      ))}
      {/* у автоматического решения рекомендации нет, пустой блок только сбивает */}
      {(card.recommendation || card.status === 'manual_review') && (
        <>
          <Typography.Title tag="h2" view="small" font="system" defaultMargins>
            Рекомендация политики
          </Typography.Title>
          {card.recommendation ? (
            <Typography.Text tag="p" view="primary-small">
              {products[card.recommendation.product]}: {money(card.recommendation.amount_kop)},{' '}
              {card.recommendation.term_months} мес., {rate(card.recommendation.rate_bp)} годовых.{' '}
              {card.recommendation.guarantor_required ? 'Нужно поручительство.' : ''}{' '}
              {card.recommendation.pledge_required ? 'Нужен залог.' : ''}
            </Typography.Text>
          ) : (
            <Typography.Text tag="p" view="primary-small">
              Рекомендация пока не поступила.
            </Typography.Text>
          )}
        </>
      )}
      <Typography.Title tag="h2" view="small" font="system" defaultMargins>
        Оценка риска
      </Typography.Title>
      {card.status === 'manual_review' && (
        <Typography.Text tag="p" view="primary-small" className="hint">
          <strong>Предварительная оценка.</strong> Факторы и расчётные условия помогают сотруднику банка принять
          решение; окончательные условия ещё не определены.
        </Typography.Text>
      )}
      <Typography.Text tag="p" view="primary-small" className="hint">
        Скоринг: оценка риска по набору факторов. Решение банка также учитывает проверки и условия продукта.
        Стоп-фактор: обстоятельство, требующее отказа или ручной проверки.
      </Typography.Text>
      {card.scoring ? (
        <>
          <Typography.Text tag="p" view="primary-small">
            Баллы: {card.scoring.score} из 100. Класс риска: {card.scoring.grade}. Вероятность невозврата:{' '}
            {card.scoring.pd}.
          </Typography.Text>
          <Typography.Text tag="p" view="primary-small">
            Версия модели: {card.scoring.version}. Версия политики: {card.policy_version ?? 'не указана'}.
          </Typography.Text>
          <div className="table-scroll">
            <Table>
              <Table.THead>
                <Table.THeadCell>Фактор</Table.THeadCell>
                <Table.THeadCell>Значение</Table.THeadCell>
                <Table.THeadCell>Баллы</Table.THeadCell>
                <Table.THeadCell>Влияние</Table.THeadCell>
              </Table.THead>
              <Table.TBody>
                {card.scoring.factors.map((factor, index) => (
                  <Table.TRow key={`${factor.code}-${index}`}>
                    <Table.TCell>{factor.name}</Table.TCell>
                    <Table.TCell>{factorValue(factor.code, factor.value)}</Table.TCell>
                    <Table.TCell>
                      {factor.points} / {factor.max_points}
                    </Table.TCell>
                    <Table.TCell>
                      {factor.points === factor.max_points
                        ? 'лучшее значение'
                        : `минус ${factor.max_points - factor.points} от максимума`}
                    </Table.TCell>
                  </Table.TRow>
                ))}
              </Table.TBody>
            </Table>
          </div>
        </>
      ) : (
        <Typography.Text tag="p" view="primary-small">
          {card.status === 'submitted' || card.status === 'checking'
            ? 'Оценка появится после проверок источников.'
            : 'Оценку не считали: решение принято по стоп-факторам или не хватило данных источников.'}
        </Typography.Text>
      )}
      {!!card.reasons.length && (
        <>
          <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
            Причины решения и стоп-факторы
          </Typography.Title>
          <ul>{reasonList}</ul>
        </>
      )}
      <Typography.Title tag="h2" view="small" font="system" defaultMargins id="bank-audit">
        История заявки
      </Typography.Title>
      {state === 'error' && <InlineNotice>Не удалось загрузить историю.</InlineNotice>}
      <Button size={40} onClick={() => setRevision((value) => value + 1)}>
        Обновить историю
      </Button>
      {state === 'ready' && !history.length && (
        <Typography.Text tag="p" view="primary-small">
          Событий пока нет.
        </Typography.Text>
      )}
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
            {event.comment && (
              <Typography.Text tag="p" view="primary-small">
                {event.comment}
              </Typography.Text>
            )}
            {!!event.reason_codes?.length && (
              <Typography.Text tag="p" view="primary-small">
                Причины: {event.reason_codes.map(reasonText).join('; ')}
              </Typography.Text>
            )}
          </li>
        ))}
      </ol>
    </section>
  );
}
