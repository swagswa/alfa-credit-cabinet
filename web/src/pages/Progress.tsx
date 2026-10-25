import { Typography } from '@alfalab/core-components/typography';
import { Button } from '../motion';
import { SteppedProgressBar } from '@alfalab/core-components/stepped-progress-bar';
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
  const stage = stages.findIndex((statuses) => statuses.includes(card.status));
  const index = stage < 0 ? stages.length : stage;
  const active = selected ?? index;
  return (
    <nav aria-label="Этапы заявки">
      <SteppedProgressBar
        maxStep={steps.length}
        step={active + 1}
        className="journey-progress-bar"
        view={steps.map((_, i) =>
          i === active
            ? 'accent'
            : i < index && !['rejected', 'expired', 'cancelled_by_client'].includes(card.status)
              ? 'positive'
              : 'secondary',
        )}
      />
      <ol className="journey-step-labels">
        {steps.map((step, i) => (
          <li key={step}>
            {onSelect ? (
              <Button
                view="text"
                size={40}
                aria-label={`${i + 1}. ${step}`}
                aria-current={i === active ? 'step' : undefined}
                aria-controls="journey-view"
                onClick={() => onSelect(i)}
              >
                {i + 1}. {step}
              </Button>
            ) : (
              <Typography.Text tag="span" view="secondary-small" aria-current={i === active ? 'step' : undefined}>
                {i + 1}. {step}
              </Typography.Text>
            )}
          </li>
        ))}
      </ol>
      {!onSelect && (
        <Typography.Text tag="p" view="primary-small" className="hint">
          {hints[card.status] ?? 'Текущий этап выделен. Следующее действие показано в карточке.'}
        </Typography.Text>
      )}
    </nav>
  );
}
