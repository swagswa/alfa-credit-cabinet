import { motion, useMotionDisabled } from '../motion';
import type { components } from '../api/schema.gen';

type Status = components['schemas']['ApplicationStatus'];
export const steps = ['Заполнение', 'Рассмотрение', 'Предложение банка', 'Документы и подпись', 'Результат'];
// статусы по этапам; всё, чего нет в списке, считается результатом
const stages: Status[][] = [
  ['draft'],
  ['submitted', 'checking', 'need_info', 'scoring', 'manual_review'],
  ['approved', 'counter_offer'],
  ['accepted', 'signing'],
];
const hints: Partial<Record<Status, string>> = {
  need_info: 'Сейчас нужен ваш ответ. Список документов и срок указаны ниже.',
  manual_review: 'Заявку рассматривает сотрудник банка. Решение появится в кабинете.',
  rejected: 'Рассмотрение завершилось отказом. Прочитайте объяснение банка ниже.',
  cancelled_by_client: 'Процесс остановлен по вашей инициативе.',
  expired: 'Процесс остановлен: срок ответа истёк.',
};
export function Progress({
  card,
  selected,
  onSelect,
}: {
  card: components['schemas']['ApplicationView'];
  selected?: number;
  onSelect?: (step: number) => void;
}) {
  const reduced = useMotionDisabled();
  const stage = stages.findIndex((statuses) => statuses.includes(card.status));
  const index = stage < 0 ? stages.length : stage;
  const active = selected ?? index;
  return (
    <nav aria-label="Этапы заявки">
      <ol className={`progress${onSelect ? ' progress-browsable' : ''}`}>
        {steps.map((step, i) => {
          const content = (
            <>
              <span aria-hidden="true">
                {i === active && (
                  <motion.i
                    className="step-selection"
                    layoutId="application-stage"
                    transition={reduced ? { duration: 0 } : undefined}
                  />
                )}
                <b>{i + 1}</b>
              </span>
              {step}
            </>
          );
          return (
            <li
              key={step}
              aria-current={!onSelect && i === active ? 'step' : undefined}
              className={
                i < index && !['rejected', 'expired', 'cancelled_by_client'].includes(card.status) ? 'complete' : ''
              }
            >
              {onSelect ? (
                <button
                  type="button"
                  aria-label={`${i + 1}. ${step}`}
                  aria-current={i === active ? 'step' : undefined}
                  aria-controls="journey-view"
                  onClick={() => onSelect(i)}
                >
                  {content}
                </button>
              ) : (
                content
              )}
            </li>
          );
        })}
      </ol>
      {!onSelect && (
        <p className="hint">{hints[card.status] ?? 'Текущий этап выделен. Следующее действие показано в карточке.'}</p>
      )}
    </nav>
  );
}
