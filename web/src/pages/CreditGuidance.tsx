import { useEffect, useState } from 'react';
import { Typography } from '@alfalab/core-components/typography';
import { api } from '../api/client';
import { money, products } from '../api/presentation';
import { Button } from '../motion';
import type { components } from '../api/schema.gen';

type Plan = components['schemas']['ImprovementPlan'];
type Estimate = components['schemas']['PreliminaryEstimate'];

export function Improvements({ id }: { id: string }) {
  const [plan, setPlan] = useState<Plan | null>(null);
  const [failed, setFailed] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    void api
      .GET('/applications/{id}/improvements', { params: { path: { id } }, signal: controller.signal })
      .then(({ data }) => {
        if (controller.signal.aborted) return;
        setPlan(data ?? null);
        setFailed(!data);
      })
      .catch(() => {
        if (!controller.signal.aborted) setFailed(true);
      });
    return () => controller.abort();
  }, [id, revision]);
  return (
    <section className="credit-guidance" aria-label="Сценарии улучшения">
      <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
        Как изменения повлияют на балл
      </Typography.Title>
      {!plan && !failed && (
        <Typography.Text tag="p" view="primary-small" role="status">
          Рассчитываем сценарии…
        </Typography.Text>
      )}
      {failed && (
        <div>
          <Typography.Text tag="p" view="primary-small">
            Сценарии сейчас недоступны. Решение по заявке сохранено.
          </Typography.Text>
          <Button
            size={32}
            view="secondary"
            onClick={() => {
              setFailed(false);
              setRevision((v) => v + 1);
            }}
          >
            Повторить расчёт
          </Button>
        </div>
      )}
      {plan && !plan.available && (
        <Typography.Text tag="p" view="primary-small">
          Для этого решения нет подходящего снимка скоринга. Числовой прогноз недоступен.
        </Typography.Text>
      )}
      {plan?.available && (
        <>
          <Typography.Text tag="p" view="primary-small">
            Сейчас: {plan.score} из 100, класс {plan.grade}. Каждый сценарий меняет только один фактор, остальные
            остаются прежними.
          </Typography.Text>
          {plan.items.length ? (
            <ul className="guidance-steps">
              {plan.items.map((step) => (
                <li key={step.input}>
                  <div className="guidance-step-heading">
                    <strong>{step.title}</strong>
                    <span className="guidance-delta">+{step.delta} баллов</span>
                  </div>
                  <Typography.Text tag="p" view="primary-small">
                    {step.detail}
                  </Typography.Text>
                  <Typography.Text tag="p" view="primary-small" className="hint">
                    Баллы: {plan.score} → {step.score} · Класс: {plan.grade} → {step.grade}
                  </Typography.Text>
                </li>
              ))}
            </ul>
          ) : (
            <Typography.Text tag="p" view="primary-small">
              Среди доступных сценариев нет изменений, которые повысят этот балл.
            </Typography.Text>
          )}
          <Typography.Text tag="p" view="primary-small" className="hint">
            Эффекты не складываются. Это прогноз балла, а не обещание одобрения: при новой заявке банк заново проверит
            данные, ограничения и условия продукта.
          </Typography.Text>
        </>
      )}
    </section>
  );
}

export function PreliminaryEstimate({ inn }: { inn: string }) {
  const [estimate, setEstimate] = useState<Estimate | null>(null);
  const [failed, setFailed] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    void api
      .GET('/companies/{inn}/preliminary', { params: { path: { inn } }, signal: controller.signal })
      .then(({ data }) => {
        if (controller.signal.aborted) return;
        setEstimate(data ?? null);
        setFailed(!data);
      })
      .catch(() => {
        if (!controller.signal.aborted) setFailed(true);
      });
    return () => controller.abort();
  }, [inn, revision]);
  return (
    <section className="preliminary-estimate" aria-label="Предварительный ориентир">
      <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
        На какую сумму ориентироваться
      </Typography.Title>
      {!estimate && !failed && (
        <Typography.Text tag="p" view="primary-small" role="status">
          Смотрим выручку в ФНС и поступления в банке…
        </Typography.Text>
      )}
      {failed && (
        <>
          <Typography.Text tag="p" view="primary-small">
            Не удалось получить данные для ориентира. Можно продолжить заполнение заявки.
          </Typography.Text>
          <Button
            type="button"
            size={32}
            view="secondary"
            onClick={() => {
              setFailed(false);
              setRevision((v) => v + 1);
            }}
          >
            Повторить оценку
          </Button>
        </>
      )}
      {estimate &&
        (estimate.items.length ? (
          <dl className="preliminary-amounts">
            {estimate.items.map((item) => (
              <div key={item.product}>
                <dt>{products[item.product]}</dt>
                <dd>до {money(item.amount_kop)}</dd>
              </div>
            ))}
          </dl>
        ) : (
          <Typography.Text tag="p" view="primary-small">
            По доступным сведениям ориентир суммы не рассчитан. Это не отказ, можно подать заявку на полную проверку.
          </Typography.Text>
        ))}
      <Typography.Text tag="p" view="primary-small" className="hint">
        Только ориентир по ФНС и данным банка. Кредитную историю не запрашиваем. Сумма не одобрена, ставка и
        окончательные условия появятся после рассмотрения заявки.
      </Typography.Text>
    </section>
  );
}
