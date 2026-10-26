import { AppLink as Link } from '../ui/AppLink';
import { Typography } from '@alfalab/core-components/typography';
import { Icon, PageHeading, SectionHeading, StatusBadge } from '../ui/Workspace';
import { useEffect, useState } from 'react';

import { api, settle } from '../api/client';
import {
  applicationGroups,
  defaultApplicationKey,
  isCompleted,
  type ApplicationGroup,
  type DashboardSnapshot,
} from '../api/dashboard';
import { money, products } from '../api/presentation';
import { useCabinetCache } from '../CabinetCache';
import { Button } from '../motion';
import type { components } from '../api/schema.gen';
import { Feed } from './Feed';

type Summary = components['schemas']['ApplicationSummary'];
type Profile = components['schemas']['Profile'];
function nextAction(status: Summary['status']) {
  if (status === 'need_info')
    return {
      title: 'Банку нужны документы',
      text: 'Посмотрите запрос и приложите нужные файлы.',
      action: 'Ответить банку',
      tone: 'amber',
    };
  if (status === 'approved')
    return {
      title: 'Заявка одобрена',
      text: 'Проверьте предложение банка и примите условия.',
      action: 'Посмотреть условия',
      tone: 'green',
    };
  if (status === 'counter_offer')
    return {
      title: 'Банк предложил другие условия',
      text: 'Сравните предложение с вашей заявкой.',
      action: 'Сравнить условия',
      tone: 'violet',
    };
  return {
    title: 'Осталось подписать документы',
    text: 'Продолжите оформление финансирования.',
    action: 'Перейти к подписанию',
    tone: 'violet',
  };
}
function nextStep(status: Summary['status']) {
  if (status === 'draft') return 'Продолжить заполнение';
  if (status === 'need_info') return 'Ответить на запрос банка';
  if (['approved', 'counter_offer'].includes(status)) return 'Посмотреть предложение';
  if (['accepted', 'signing'].includes(status)) return 'Перейти к подписанию';
  if (isCompleted(status)) return 'Посмотреть результат';
  return 'Посмотреть ход рассмотрения';
}

export function Dashboard() {
  const cache = useCabinetCache();
  const [snapshot, setSnapshot] = useState<DashboardSnapshot | undefined>(() => cache.get('dashboard'));
  const [profile, setProfile] = useState<Profile | undefined>(() => cache.get('profile'));
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    if (cache.get('products')) return;
    return settle(api.GET('/products'), ({ data }) => {
      if (data?.length) cache.set('products', data);
    });
  }, [cache]);
  useEffect(() => {
    let active = true;
    let serial = 0;
    let pending = false;
    async function load() {
      const request = ++serial;
      pending = true;
      try {
        const groups = Object.keys(applicationGroups) as ApplicationGroup[];
        const [page, attention, ...counts] = await Promise.all([
          api.GET('/applications', { params: { query: { limit: 20, offset: 0, sort: '-created_at' } } }),
          api.GET('/applications', {
            params: {
              query: {
                limit: 3,
                sort: 'due_at',
                status: ['need_info', 'approved', 'counter_offer', 'accepted', 'signing'],
              },
            },
          }),
          ...groups.map((group) =>
            api.GET('/applications', { params: { query: { limit: 1, status: applicationGroups[group].statuses } } }),
          ),
        ]);
        if (!active || request !== serial) return;
        if (!page.data || !attention.data || counts.some((result) => !result.data)) {
          setError('Не удалось обновить обзор. Все заявки доступны в разделе «Мои заявки».');
          return;
        }
        const data: DashboardSnapshot = {
          page: page.data,
          attention: attention.data.items,
          counts: Object.fromEntries(
            groups.map((group, index) => [group, counts[index].data!.total]),
          ) as DashboardSnapshot['counts'],
        };
        cache.set('dashboard', data);
        cache.set(defaultApplicationKey, page.data);
        setSnapshot(data);
        setError('');
      } catch {
        if (active && request === serial) setError('Обзор временно недоступен. Попробуйте обновить его.');
      } finally {
        if (request === serial) pending = false;
      }
    }
    void load();
    const stream = new EventSource('/api/events');
    stream.onopen = () => {
      if (!pending) void load();
    };
    stream.onerror = () => {
      void api.GET('/me').catch(() => undefined);
    };
    const changed = () => {
      void load();
      window.dispatchEvent(new Event('feed-refresh'));
    };
    stream.addEventListener('status', changed);
    stream.addEventListener('document', changed);
    window.addEventListener('application-refresh', changed);
    return () => {
      active = false;
      stream.close();
      window.removeEventListener('application-refresh', changed);
    };
  }, [cache, revision]);
  useEffect(() => {
    let active = true;
    settle(
      api.GET('/profile'),
      ({ data }) => {
        if (active && data) {
          cache.set('profile', data);
          setProfile(data);
        }
      },
      () => undefined,
    );
    return () => {
      active = false;
    };
  }, [cache, revision]);
  const recent = snapshot?.page.items.slice(0, 3) ?? [];
  const activeLoans = profile?.companies.flatMap((company) =>
    company.loans
      .filter((loan) => ['issued', 'closed'].includes(loan.status))
      .map((loan) => ({ company, loan })),
  ) ?? [];
  const groups = snapshot
    ? (Object.keys(applicationGroups) as ApplicationGroup[]).filter(
        (group) => group !== 'completed' && group !== 'response' && group !== 'drafts' && snapshot.counts[group] > 0,
      )
    : [];
  return (
    <div className="dashboard">
      <PageHeading title="Обзор" description="Заявки, решения банка и ваши следующие шаги" />
      <section className="welcome dashboard-promo" aria-labelledby="funding-promo-title">
        <div>
          <Typography.Title tag="h2" view="small" font="system" defaultMargins id="funding-promo-title">
            Деньги на планы
            <br />
            вашей компании
          </Typography.Title>
          <Typography.Text tag="p" view="primary-small">
            На закупки, оборудование и развитие бизнеса.
            <br />
            Подайте заявку и следите за решением в кабинете.
          </Typography.Text>
          <Link className="promo-link" to="/client/new">
            Подать заявку <span aria-hidden="true">↗</span>
          </Link>
        </div>
        <div className="welcome-art" aria-hidden="true">
          <span>₽</span>
          <small>
            На новый этап
            <br />
            вашего бизнеса
          </small>
        </div>
      </section>
      {error && (
        <div className="dashboard-error" role="alert">
          <Typography.Text tag="p" view="primary-small">
            {error}
          </Typography.Text>
          <Button size={40} onClick={() => setRevision((value) => value + 1)}>
            Обновить обзор
          </Button>
          <Link to="/client/applications">К заявкам →</Link>
        </div>
      )}
      {!snapshot && !error && (
        <div className="dashboard-loading" role="status">
          <span />
          Загружаем ваш обзор…
        </div>
      )}
      {snapshot && snapshot.page.total === 0 && (
        <section className="dashboard-onboarding">
          <div className="onboarding-symbol" aria-hidden="true">
            ↗
          </div>
          <Typography.Title tag="h2" view="small" font="system" defaultMargins>
            Начните с первой заявки
          </Typography.Title>
          <Typography.Text tag="p" view="primary-small">
            Расскажите о планах бизнеса. Подберите финансирование и пройдите весь путь до решения банка в одном
            кабинете.
          </Typography.Text>
          <ol>
            <li>
              <b>01</b> Заполните анкету
            </li>
            <li>
              <b>02</b> Получите решение
            </li>
            <li>
              <b>03</b> Примите условия и подпишите документы
            </li>
          </ol>
        </section>
      )}
      {!!snapshot?.attention.length && (
        <section className="dashboard-attention" aria-labelledby="attention-heading">
          <div className="dashboard-section-heading">
            <Typography.Title tag="h2" view="small" font="system" id="attention-heading">
              Требует внимания
            </Typography.Title>
            <span>Ваш следующий шаг</span>
          </div>
          <div className="attention-grid">
            {snapshot.attention.map((item) => {
              const action = nextAction(item.status);
              return (
                <article className={`attention-card attention-${action.tone}`} key={item.id}>
                  <div className="attention-top">
                    <span className="attention-symbol" aria-hidden="true">
                      {item.status === 'approved' ? <Icon name="check" /> : '↗'}
                    </span>
                    <span>Заявка № {item.number}</span>
                  </div>
                  <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
                    {action.title}
                  </Typography.Title>
                  <Typography.Text tag="p" view="primary-small">
                    {action.text}
                  </Typography.Text>
                  {item.due_at && (
                    <Typography.Text tag="p" view="primary-small" className="attention-deadline">
                      Ответить до{' '}
                      {new Date(item.due_at).toLocaleString('ru-RU', {
                        day: 'numeric',
                        month: 'long',
                        hour: '2-digit',
                        minute: '2-digit',
                      })}
                    </Typography.Text>
                  )}
                  <Link to={`/client/applications/${item.id}`}>
                    {action.action}
                    <span aria-hidden="true">↗</span>
                  </Link>
                </article>
              );
            })}
          </div>
        </section>
      )}
      {!!groups.length && (
        <nav className="dashboard-metrics" aria-label="Сводка заявок">
          {groups.map((group) => (
            <Link key={group} to={`/client/applications?group=${group}`} className={`metric-${group}`}>
              <span>{applicationGroups[group].label}</span>
              <strong>{snapshot!.counts[group]}</strong>
              <span className="metric-arrow" aria-hidden="true">
                ↗
              </span>
            </Link>
          ))}
        </nav>
      )}
      {!!activeLoans.length && (
        <section className="dashboard-loans" aria-labelledby="dashboard-loans-heading">
          <SectionHeading title={<span id="dashboard-loans-heading">Действующие кредиты</span>}>
            <Link to="/client/profile">Все кредиты →</Link>
          </SectionHeading>
          <ul className="dashboard-loan-list">
            {activeLoans.map(({ company, loan }) => (
                <li className="dashboard-loan" key={loan.application_id}>
                  <div className="dashboard-loan-main">
                    <span className="dashboard-loan-company">{company.company.short_name}</span>
                    <Link to={`/client/applications/${loan.application_id}`}>Заявка № {loan.number}</Link>
                  </div>
                  <div className="dashboard-loan-payment">
                    <strong>{money(loan.monthly_payment_kop, true)}</strong>
                    <span>в месяц · {loan.term_months} мес.</span>
                  </div>
                </li>
            ))}
          </ul>
        </section>
      )}
      {!!recent.length && (
        <section className="dashboard-recent" aria-labelledby="recent-heading">
          <SectionHeading title={<span id="recent-heading">Актуальные заявки</span>}>
            <Link to="/client/applications">Все заявки →</Link>
          </SectionHeading>
          <ul className="dashboard-application-grid">
            {recent.map((item) => (
              <li key={item.id} className="dashboard-application">
                <div className="recent-card-top">
                  <span>№ {item.number}</span>
                  <StatusBadge status={item.status} />
                </div>
                <Typography.Text tag="p" view="primary-small" className="dashboard-company">
                  {item.company.short_name}
                </Typography.Text>
                <small>Запрошенная сумма</small>
                <strong className="dashboard-amount">{money(item.amount_kop, true)}</strong>
                <Typography.Text tag="p" view="primary-small" className="dashboard-product">
                  {products[item.product]} · {item.term_months} мес.
                </Typography.Text>
                <Link to={`/client/applications/${item.id}`}>
                  {nextStep(item.status)}
                  <span aria-hidden="true">→</span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}
      <Feed />
    </div>
  );
}
