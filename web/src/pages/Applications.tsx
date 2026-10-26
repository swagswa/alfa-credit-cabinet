import { AppLink as Link } from '../ui/AppLink';
import { NeedFromText } from './NeedFromText';
import { Typography } from '@alfalab/core-components/typography';
import { InlineNotice } from '../ui/InlineNotice';
import { Disclosure } from '../ui/Disclosure';
import { CheckProgress } from './CheckProgress';
import { PreliminaryEstimate } from './CreditGuidance';
import { parseCheckEvent, updateCheckProgress, type CheckProgressState } from '../api/checkProgress';
import {
  goals,
  kinds,
  kop,
  labels,
  money,
  options,
  products as productNames,
  rate,
  representatives,
  requiredDocs,
} from '../api/presentation';
import { Icon, PageHeading, StatusBadge, FieldPair } from '../ui/Workspace';
import { useCabinetCache } from '../CabinetCache';
import { applicationGroups, isApplicationGroup, isCompleted } from '../api/dashboard';
import { CompletedJourney } from './CompletedJourney';
import { Value } from '../motion/Value';
import { motion, AnimatePresence, Button, Checkbox, ListItem, Select, StatusText, useMotionDisabled } from '../motion';
import { BottomNotice, dismiss, notify } from '../BottomNotice';
import { Progress } from './Progress';
import { ApplicationDeadline } from '../ui/Deadline';
import { MoneyField } from '../ui/MoneyField';
import { ProductChoices } from '../ui/ProductChoices';
import { BankDetails } from './BankDetails';
import { BankQueueOffer } from './BankQueueOffer';
import { Cancel, Decision } from './Decision';
import { Documents } from './Documents';
import { useEffect, useState, useRef, type FormEvent, type SVGProps } from 'react';
import { useLocation, useNavigate, useParams, useSearchParams } from 'react-router';
import type { RepeatApplication } from '../api/repeatApplication';
import { Input } from '@alfalab/core-components/input';
import { IconButton } from '@alfalab/core-components/icon-button';
import { ArrowLeftSIcon } from '@alfalab/icons-glyph/ArrowLeftSIcon';
import { ArrowRightSIcon } from '@alfalab/icons-glyph/ArrowRightSIcon';
import { api, errorText, settle } from '../api/client';
import type { components, operations } from '../api/schema.gen';

type App = components['schemas']['ApplicationView'];
// у глифа role=img без подписи, кнопка и так подписана
const PrevIcon = (props: SVGProps<SVGSVGElement>) => <ArrowLeftSIcon {...props} aria-hidden="true" />;
const NextIcon = (props: SVGProps<SVGSVGElement>) => <ArrowRightSIcon {...props} aria-hidden="true" />;
type Sort = NonNullable<operations['listApplications']['parameters']['query']>['sort'];
const sorts = [
  { key: '-created_at', content: 'Сначала новые' },
  { key: 'created_at', content: 'Сначала старые' },
  { key: '-updated_at', content: 'По последнему изменению' },
  { key: 'updated_at', content: 'Давно не обновлялись' },
  { key: '-amount_kop', content: 'Сначала большая сумма' },
  { key: 'amount_kop', content: 'Сначала меньшая сумма' },
  { key: 'due_at', content: 'По ближайшему сроку' },
];
const bankSorts = [...sorts, { key: 'score', content: 'Сначала рискованные' }];

export function Applications({
  bank = false,
  createOnly = false,
  userId,
  role,
}: {
  bank?: boolean;
  createOnly?: boolean;
  userId?: string;
  role?: components['schemas']['Role'];
}) {
  const reduced = useMotionDisabled();
  const cache = useCabinetCache();
  const location = useLocation();
  const repeat = createOnly
    ? (location.state as { repeatApplication?: RepeatApplication } | null)?.repeatApplication
    : undefined;
  const storageKey = createOnly && userId ? `credit:new:v1:${userId}` : '';
  const [saved] = useState<Record<string, string | boolean>>(() => {
    if (!storageKey) return {};
    try {
      const value: unknown = JSON.parse(sessionStorage.getItem(storageKey) ?? '{}');
      if (!value || typeof value !== 'object' || Array.isArray(value)) return {};
      if (('_source' in value ? value._source : '') !== (repeat ? location.key : '')) return {};
      return Object.fromEntries(
        Object.entries(value).filter(([, v]) => typeof v === 'string' || typeof v === 'boolean'),
      );
    } catch {
      return {};
    }
  });
  const text = (key: string, fallback: string) => (typeof saved[key] === 'string' ? (saved[key] as string) : fallback);
  const draftCleared = useRef(false);
  const { id } = useParams();
  const navigate = useNavigate();
  const [search, setSearch] = useSearchParams();
  const requestedGroup = search.get('group');
  const group = isApplicationGroup(requestedGroup) ? requestedGroup : undefined;
  const root = bank ? '/bank' : '/client';
  const cached = cache.get<components['schemas']['ApplicationPage']>(
    JSON.stringify(['applications', bank, 0, group ? `group:${group}` : null, null, null, '-created_at']),
  );
  const [items, setItems] = useState<components['schemas']['ApplicationSummary'][]>(cached?.items ?? []);
  const [card, setCard] = useState<App | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(!createOnly && (!!id || !cached));
  const [busy, setBusy] = useState(false);
  const [checkProgress, setCheckProgress] = useState<CheckProgressState>({ applicationId: '', version: 0, items: {} });
  const [connectionFailed, setConnectionFailed] = useState(false);
  const [revision, setRevision] = useState(0);
  const [status, setStatus] = useState<components['schemas']['ApplicationStatus'] | undefined>();
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState(cached?.total ?? 0);
  const [product, setProduct] = useState<components['schemas']['ProductCode'] | undefined>();
  const [grade, setGrade] = useState<components['schemas']['Grade'] | undefined>();
  const [sort, setSort] = useState<Sort>('-created_at');
  const [inn, setInn] = useState(text('inn', repeat?.company.inn ?? ''));
  const [amount, setAmount] = useState(text('amount', repeat ? String(repeat.amount_kop / 100) : ''));
  const [term, setTerm] = useState(text('term', String(repeat?.term_months ?? 24)));
  const [consent, setConsent] = useState(false);
  const [personalConsent, setPersonalConsent] = useState(false);
  const [proxy, setProxy] = useState(
    typeof saved.proxy === 'boolean' ? saved.proxy : repeat?.representative_role === 'proxy',
  );
  const [company, setCompany] = useState<components['schemas']['Company'] | null>(null);
  const [lookupBusy, setLookupBusy] = useState(false);
  const [own, setOwn] = useState<components['schemas']['CompanyRef'][]>([]);
  const [registryDown, setRegistryDown] = useState(false);
  const [chosenProduct, setChosenProduct] = useState<components['schemas']['ProductCode']>(
    text('chosenProduct', repeat?.product ?? 'working_capital') as components['schemas']['ProductCode'],
  );
  const [goal, setGoal] = useState<components['schemas']['Goal']>(
    text('goal', repeat?.purpose ?? 'purchase') as components['schemas']['Goal'],
  );
  const [revenue, setRevenue] = useState(
    text('revenue', repeat?.monthly_revenue_kop !== undefined ? String(repeat.monthly_revenue_kop / 100) : ''),
  );
  const [equipmentCost, setEquipmentCost] = useState(
    text('equipmentCost', repeat?.equipment_cost_kop !== undefined ? String(repeat.equipment_cost_kop / 100) : ''),
  );
  const [recommendation, setRecommendation] = useState<components['schemas']['Recommendation'] | null>(null);
  const [recommendBusy, setRecommendBusy] = useState(false);
  const [products, setProducts] = useState<components['schemas']['Product'][]>(cache.get('products') ?? []);
  const [catalogError, setCatalogError] = useState('');
  useEffect(() => {
    if (!storageKey || draftCleared.current) return;
    try {
      sessionStorage.setItem(
        storageKey,
        JSON.stringify({
          _source: repeat ? location.key : '',
          inn,
          amount,
          term,
          proxy,
          chosenProduct,
          goal,
          revenue,
          equipmentCost,
        }),
      );
    } catch {
      /* Ограничение хранилища не должно блокировать анкету. */
    }
  }, [storageKey, repeat, location.key, inn, amount, term, proxy, chosenProduct, goal, revenue, equipmentCost]);
  const selectedProduct = products.find((item) => item.code === chosenProduct);
  // своя компания уже есть в кабинете: поиск в реестре только добавляет сведения и черновик не блокирует
  const known = own.find((item) => item.inn === inn);
  const found = company?.inn === inn ? company : known;
  useEffect(() => {
    if (!createOnly) return;
    return settle(api.GET('/me'), ({ data }) => {
      if (!data) return;
      setOwn(data.companies.map((item) => item.company));
      if (data.companies.length === 1) setInn((value) => value || data.companies[0].company.inn);
    });
  }, [createOnly]);
  const cardProduct = products.find((item) => item.code === card?.product);
  const warning = card ? submitWarning(card, cardProduct) : '';
  useEffect(() => {
    if (bank) return;
    return settle(
      api.GET('/products'),
      ({ data }) => {
        if (data?.length) {
          cache.set('products', data);
          setProducts(data);
        } else setCatalogError('Не удалось загрузить продукты. Обновите страницу.');
      },
      () => setCatalogError('Нет связи с каталогом продуктов. Обновите страницу.'),
    );
  }, [bank, cache]);
  const submitted = useRef('');
  // решение приходит за доли секунды, «отправлена» поверх него только мешает
  useEffect(() => {
    if (submitted.current && card && !['submitted', 'checking', 'scoring'].includes(card.status)) {
      dismiss(submitted.current);
      submitted.current = '';
    }
  }, [card]);
  const lookupVersion = useRef(0);
  const recommendationVersion = useRef(0);
  const key = useRef<string | null>(null);
  const loaded = useRef(!id && !!cached);
  async function lookup() {
    const version = ++lookupVersion.current;
    setLookupBusy(true);
    setError('');
    setCompany(null);
    setRegistryDown(false);
    try {
      const { data, error, response } = await api.GET('/companies/{inn}', { params: { path: { inn } } });
      if (version !== lookupVersion.current) return;
      if (data) setCompany(data);
      else if (response.status >= 500) {
        setRegistryDown(true);
        if (!known) setError('Реестр сейчас не отвечает. Попробуйте через минуту.');
      } else setError(errorText(error, 'Не удалось найти компанию.'));
    } catch {
      if (version !== lookupVersion.current) return;
      setRegistryDown(true);
      if (!known) setError('Нет связи с реестром. Попробуйте ещё раз.');
    } finally {
      if (version === lookupVersion.current) setLookupBusy(false);
    }
  }
  function invalidateRecommendation() {
    recommendationVersion.current++;
    key.current = null;
    setRecommendation(null);
  }
  async function recommend() {
    const version = ++recommendationVersion.current;
    setRecommendBusy(true);
    setError('');
    try {
      const { data, error } = await api.GET('/products/recommendation', {
        params: {
          query: {
            goal,
            amount_kop: kop(amount),
            term_months: Number(term),
            ...(revenue ? { monthly_revenue_kop: kop(revenue) } : {}),
          },
        },
      });
      if (version !== recommendationVersion.current) return;
      if (data) {
        if (!data.items.length) {
          setRecommendation(null);
          setError('Для этих параметров нет подходящих продуктов. Измените сумму или срок.');
          return;
        }
        setRecommendation(data);
        // без этого в анкете оставался оборотный кредит, а поле стоимости оборудования не появлялось
        const primary = data.items.find((item) => item.primary) ?? data.items[0];
        setChosenProduct(primary.product);
        setAmount(String(primary.amount_kop / 100));
        setTerm(String(primary.term_months));
        key.current = null;
      } else setError(errorText(error, 'Не удалось подобрать продукт.'));
    } catch {
      setError('Нет связи с сервером. Попробуйте ещё раз.');
    } finally {
      setRecommendBusy(false);
    }
  }
  useEffect(() => {
    if (createOnly) return;
    let active = true;
    let serial = 0;
    async function load() {
      const request = ++serial;
      if (!loaded.current) setLoading(true);
      try {
        const result = id
          ? await api.GET('/applications/{id}', { params: { path: { id } } })
          : await api.GET('/applications', {
              params: {
                query: {
                  limit: 20,
                  offset,
                  status: status ? [status] : group ? applicationGroups[group].statuses : undefined,
                  product,
                  grade: grade ? [grade] : undefined,
                  sort,
                },
              },
            });
        if (!active || request !== serial) return;
        if (!result.data) {
          if (!loaded.current || id) {
            setCard(null);
            setItems([]);
          }
          setError(
            result.response.status === 401
              ? 'Сессия завершена. Выйдите и войдите снова.'
              : result.response.status === 404
                ? 'Заявка не найдена.'
                : 'Не удалось загрузить заявки.',
          );
          return;
        }
        setError('');
        loaded.current = true;
        if ('items' in result.data) {
          cache.set(
            JSON.stringify([
              'applications',
              bank,
              offset,
              status ?? (group ? `group:${group}` : null),
              product ?? null,
              grade ?? null,
              sort,
            ]),
            result.data,
          );
          setItems(result.data.items);
          setTotal(result.data.total);
        } else {
          cache.rememberApplication(result.data);
          setCard(result.data);
        }
      } catch {
        if (active && request === serial) setError('Нет связи с сервером. Попробуйте обновить данные.');
      } finally {
        if (active && request === serial) {
          setLoading(false);
        }
      }
    }
    window.addEventListener('application-refresh', load);
    void load();
    return () => {
      active = false;
      window.removeEventListener('application-refresh', load);
    };
  }, [id, revision, offset, status, product, grade, sort, bank, cache, createOnly, group]);
  // поток событий не зависит от фильтров: фильтры читает load через событие
  useEffect(() => {
    if (createOnly) return;
    const reload = () => window.dispatchEvent(new Event('application-refresh'));
    const events = new EventSource('/api/events');
    events.onopen = () => {
      setCheckProgress({ applicationId: id ?? '', version: 0, items: {} });
      setConnectionFailed(false);
      reload();
      window.dispatchEvent(new Event('feed-refresh'));
    };
    events.onerror = () => {
      setConnectionFailed(true);
      void api.GET('/me').catch(() => undefined);
    };
    const check = (message: MessageEvent<string>) => {
      const event = parseCheckEvent(message.data);
      if (!event || event.application_id !== id) return;
      setCheckProgress((current) => updateCheckProgress(current, event));
      // Результат сохранён в БД: обновляем также подробные ответы источников у банка.
      if (event.check_status) reload();
    };
    const statusChanged = (message: MessageEvent<string>) => {
      try {
        const event = JSON.parse(message.data);
        if (id && event.application_id !== id) return;
        if (id && event.status === 'checking' && Number.isSafeInteger(event.version)) {
          setCheckProgress((current) =>
            current.applicationId !== id || event.version > current.version
              ? { applicationId: id ?? '', version: event.version, items: {} }
              : current,
          );
        }
      } catch {
        /* Повреждённое уведомление не должно блокировать обновление карточки. */
      }
      reload();
    };
    events.addEventListener('check', check);
    events.addEventListener('status', statusChanged);
    const updateFeed = () => window.dispatchEvent(new Event('feed-refresh'));
    events.addEventListener('status', updateFeed);
    events.addEventListener('document', reload);
    return () => events.close();
  }, [id, revision, createOnly]);
  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || !found || !selectedProduct) return;
    const amountKop = kop(amount);
    if (
      !Number.isSafeInteger(amountKop) ||
      amountKop < selectedProduct.amount_min_kop ||
      amountKop > selectedProduct.amount_max_kop ||
      !Number.isInteger(Number(term)) ||
      Number(term) < selectedProduct.term_min_months ||
      Number(term) > selectedProduct.term_max_months
    ) {
      setError('Сумма или срок не подходят выбранному продукту. Проверьте указанные диапазоны.');
      return;
    }
    setBusy(true);
    setError('');
    key.current ??= crypto.randomUUID();
    try {
      const { data, error: problem } = await api.POST('/applications', {
        params: { header: { 'Idempotency-Key': key.current } },
        body: {
          inn,
          amount_kop: amountKop,
          term_months: Number(term),
          product: chosenProduct,
          purpose: goal,
          representative_role: proxy ? 'proxy' : found.kind === 'IP' ? 'owner_ip' : 'director',
          bki_consent: consent,
          pd_consent: personalConsent,
          ...(revenue ? { monthly_revenue_kop: kop(revenue) } : {}),
          ...(chosenProduct === 'equipment' && equipmentCost ? { equipment_cost_kop: kop(equipmentCost) } : {}),
        },
      });
      if (!data) {
        setError(errorText(problem, 'Не удалось сохранить черновик.'));
        return;
      }
      cache.rememberApplication(data, true);
      draftCleared.current = true;
      try {
        if (storageKey) sessionStorage.removeItem(storageKey);
      } catch {
        /* storage unavailable */
      }
      notify('Черновик создан.');
      key.current = null;
      setAmount('');
      navigate(root + '/applications/' + data.id);
    } catch {
      setError('Нет связи с сервером. Повторите сохранение с теми же данными.');
    } finally {
      setBusy(false);
    }
  }
  async function submit() {
    if (!card || busy) return;
    setBusy(true);
    setError('');
    try {
      const { data, error: problem } = await api.POST('/applications/{id}/submit', {
        params: { path: { id: card.id } },
      });
      if (data) {
        cache.rememberApplication(data);
        setCard(data);
        submitted.current = notify('Заявка отправлена в банк.');
      } else setError(errorText(problem, 'Не удалось отправить заявку.'));
    } catch {
      setError('Нет связи с сервером. Проверьте статус перед повторной отправкой.');
    } finally {
      setBusy(false);
    }
  }
  const [title, description] = bank
    ? id
      ? ['Заявка клиента', card?.company.short_name ?? '']
      : ['Очередь банка', 'Заявки клиентов, проверки и решения']
    : createOnly
      ? ['Новая заявка', 'Заполните анкету и подберите финансирование']
      : ['Мои заявки', 'Заявки, документы и решения банка'];
  return (
    <div className="workspace-page">
      <PageHeading
        description={description}
        title={title}
        back={id ? (bank ? root : '/client/applications') : undefined}
      >
        <div className="page-actions">
          <Button
            className={createOnly || (!connectionFailed && !error) ? 'refresh-hidden' : undefined}
            tabIndex={createOnly || (!connectionFailed && !error) ? -1 : undefined}
            aria-hidden={createOnly || (!connectionFailed && !error) || undefined}
            aria-label="Обновить"
            size={40}
            onClick={() => setRevision((value) => value + 1)}
          >
            <Icon name="refresh" />
          </Button>
        </div>
      </PageHeading>
      <section className="workspace-content" aria-busy={loading}>
        {error && <BottomNotice message={error} onClose={() => setError('')} />}
        {(loading || (!!id && card?.id !== id && !error)) && !createOnly && (
          <div className="initial-loading" role="status">
            Загружаем...
          </div>
        )}
        {id ? (
          <>
            {card?.id === id && (
              <article>
                {!bank && isCompleted(card.status) ? (
                  <CompletedJourney key={card.id} card={card} />
                ) : (
                  <>
                    <Progress card={card} bank={bank} />
                    <Typography.Title tag="h2" view="small" font="system" defaultMargins>
                      Заявка № {card.number}
                    </Typography.Title>
                    <ApplicationDeadline card={card} />
                    <Typography.Text tag="p" view="primary-small">
                      {card.company.short_name}, ИНН {card.company.inn}
                    </Typography.Text>
                    <Typography.Text tag="p" view="primary-small">
                      <StatusText value={card.status}>{labels[card.status]}</StatusText>
                    </Typography.Text>
                    <Typography.Text tag="p" view="primary-small">
                      {card.status === 'counter_offer' ? 'Запрошено: ' : ''}
                      {money(card.amount_kop)}, {card.term_months} мес.
                    </Typography.Text>
                    {!bank && (
                      <Typography.Text tag="p" view="primary-small">
                        {
                          {
                            draft: 'Проверьте сведения и отправьте заявку в банк.',
                            submitted: 'Заявка получена банком.',
                            checking: 'Проверяем сведения. Пока от вас ничего не требуется.',
                            scoring: 'Оцениваем условия финансирования.',
                            need_info: 'Ответьте на запрос банка ниже.',
                            manual_review: 'Сотрудник банка рассматривает заявку.',
                            approved: 'Проверьте и примите условия ниже.',
                            counter_offer: 'Сравните предложенные условия с вашим запросом.',
                            accepted: 'Готовим документы для подписания.',
                            signing: 'Получите код и подпишите договор.',
                            rejected: 'Банк не одобрил заявку. Причины указаны ниже.',
                            issued: 'Выдача завершена.',
                            closed: 'Заявка завершена.',
                            cancelled_by_client: 'Заявка отменена.',
                            expired: 'Срок действия заявки истёк.',
                          }[card.status]
                        }
                      </Typography.Text>
                    )}
                    {!bank && card.status === 'draft' && (
                      <Disclosure className="edit-panel" label={<>Изменить данные заявки</>}>
                        <DraftEditor
                          key={card.id}
                          card={card}
                          onSaved={(data) => {
                            cache.rememberApplication(data);
                            setCard(data);
                          }}
                        />
                      </Disclosure>
                    )}
                    {!bank && card.allowed_next.includes('submitted') && (
                      <>
                        {warning && (
                          <Typography.Text tag="p" view="primary-small" role="note">
                            {warning}
                          </Typography.Text>
                        )}
                        <Button view="primary" loading={busy} onClick={submit}>
                          Отправить в банк
                        </Button>
                      </>
                    )}
                    <CheckProgress card={card} bank={bank} live={checkProgress} disconnected={connectionFailed} />
                    {!bank && <Decision key={card.id} card={card} refresh={() => setRevision((value) => value + 1)} />}
                    {bank && <BankDetails key={`bank-${card.id}`} card={card} role={role} />}
                    <Documents
                      key={`documents-${card.id}`}
                      card={card}
                      bank={bank}
                      product={cardProduct}
                      refresh={() => setRevision((value) => value + 1)}
                    />
                    {!bank && (
                      <Cancel key={`cancel-${card.id}`} card={card} refresh={() => setRevision((value) => value + 1)} />
                    )}
                  </>
                )}
              </article>
            )}
          </>
        ) : (
          <>
            {!createOnly && (
              <>
                <div className="filters">
                  <Select
                    block
                    label="Статус заявки"
                    options={[
                      { key: 'all', content: 'Все статусы' },
                      ...Object.entries(applicationGroups)
                        .filter(([, value]) => !bank || value.bank)
                        .map(([key, value]) => ({
                          key: `group:${key}`,
                          content: bank ? value.bank : value.label,
                        })),
                      ...options(labels).filter((option) => !bank || option.key !== 'draft'),
                    ]}
                    selected={status ?? (group ? `group:${group}` : 'all')}
                    onChange={({ selected }) => {
                      setOffset(0);
                      const key = selected?.key;
                      if (key?.startsWith('group:')) {
                        setStatus(undefined);
                        setSearch({ group: key.slice(6) });
                      } else {
                        setStatus(key === 'all' ? undefined : (key as components['schemas']['ApplicationStatus']));
                        setSearch({});
                      }
                    }}
                  />
                  {bank && (
                    <>
                      <Select
                        block
                        label="Продукт"
                        options={[{ key: 'all', content: 'Все продукты' }, ...options(productNames)]}
                        selected={product ?? 'all'}
                        onChange={({ selected }) => {
                          setOffset(0);
                          setProduct(
                            selected?.key === 'all'
                              ? undefined
                              : (selected?.key as components['schemas']['ProductCode']),
                          );
                        }}
                      />
                      <Select
                        block
                        label="Класс риска"
                        options={[
                          { key: 'all', content: 'Все классы' },
                          ...['A', 'B', 'C', 'D', 'E'].map((key) => ({ key, content: `Класс ${key}` })),
                        ]}
                        selected={grade ?? 'all'}
                        onChange={({ selected }) => {
                          setOffset(0);
                          setGrade(
                            selected?.key === 'all' ? undefined : (selected?.key as components['schemas']['Grade']),
                          );
                        }}
                      />
                    </>
                  )}
                  <Select
                    block
                    label="Порядок заявок"
                    options={bank ? bankSorts : sorts}
                    selected={sort}
                    onChange={({ selected }) => {
                      setOffset(0);
                      setSort((selected?.key ?? '-created_at') as Sort);
                    }}
                  />
                </div>
                {!loading && (
                  <>
                    {!items.length && (
                      <Typography.Text tag="p" view="primary-small">
                        {status || product || grade || group ? 'По выбранным условиям заявок нет.' : 'Заявок пока нет.'}
                      </Typography.Text>
                    )}
                    <motion.ul layout={reduced ? false : 'position'} className="applications-list">
                      <AnimatePresence initial={false}>
                        {items.map((item) => (
                          <ListItem className="application-card" key={item.id}>
                            <Link to={`${root}/applications/${item.id}`}>
                              № {item.number}: {item.company.short_name}
                            </Link>
                            <StatusBadge status={item.status} />
                            {bank && item.status === 'counter_offer' ? (
                              <BankQueueOffer key={item.id + item.updated_at} item={item} />
                            ) : (
                              <>
                                <strong className="application-amount">
                                  <Value text={money(item.amount_kop)} />
                                </strong>
                                <span>{item.term_months} мес.</span>{' '}
                              </>
                            )}
                          </ListItem>
                        ))}
                      </AnimatePresence>
                    </motion.ul>
                    <Typography.Text tag="p" view="primary-small">
                      Всего заявок: {total}. Страница {Math.floor(offset / 20) + 1}
                    </Typography.Text>
                    <nav className="application-pagination" aria-label="Страницы заявок">
                      <IconButton
                        icon={PrevIcon}
                        size={40}
                        view="secondary"
                        aria-label="Назад"
                        disabled={offset === 0}
                        onClick={() => setOffset(Math.max(0, offset - 20))}
                      />
                      <IconButton
                        icon={NextIcon}
                        size={40}
                        view="secondary"
                        aria-label="Далее"
                        disabled={offset + 20 >= total}
                        onClick={() => setOffset(offset + 20)}
                      />
                    </nav>
                  </>
                )}
              </>
            )}
            {!bank && createOnly && (
              <form
                className="application-form"
                onSubmit={create}
                onChange={() => {
                  key.current = null;
                }}
              >
                {!!Object.keys(saved).length && (
                  <Typography.Text tag="p" view="primary-small" role="status" className="hint">
                    Анкета восстановлена в этой вкладке. Подтвердите согласия перед сохранением.
                  </Typography.Text>
                )}
                {repeat && (
                  <Typography.Text tag="p" view="primary-small" className="repeat-caption">
                    Данные перенесены из заявки № {repeat.number}. Проверьте их, предоставьте согласия и приложите
                    документы заново.
                  </Typography.Text>
                )}
                <Typography.Title tag="h2" view="small" font="system" defaultMargins>
                  1. Компания
                </Typography.Title>
                <Typography.Text tag="p" view="primary-small" className="hint">
                  Найдём сведения по ИНН. Затем укажите цель и параметры кредита.
                </Typography.Text>
                <Input
                  block
                  label="ИНН компании"
                  value={inn}
                  onChange={(event) => {
                    lookupVersion.current++;
                    setLookupBusy(false);
                    setCompany(null);
                    setRegistryDown(false);
                    key.current = null;
                    setInn(event.target.value.replace(/\D/g, ''));
                  }}
                  onKeyDown={(event) => {
                    // Enter в поле ИНН ищет компанию, а не отправляет анкету
                    if (event.key !== 'Enter') return;
                    event.preventDefault();
                    if (/^\d{10}(\d{2})?$/.test(inn)) void lookup();
                  }}
                  inputMode="numeric"
                  maxLength={12}
                  required
                  disabled={busy}
                />
                <Button
                  type="button"
                  loading={lookupBusy}
                  disabled={busy || !/^\d{10}(\d{2})?$/.test(inn)}
                  onClick={lookup}
                >
                  Найти компанию
                </Button>
                {!company && known && (
                  <div className="company-result">
                    <span className="hint">Ваша компания в кабинете</span>
                    <br />
                    <strong>{known.short_name}</strong>
                    <Typography.Text tag="p" view="primary-small">
                      {registryDown
                        ? 'Реестр сейчас не отвечает, продолжим с данными из кабинета. Банк проверит компанию после подачи.'
                        : 'Можно продолжать. Сведения из реестра покажем по кнопке «Найти компанию».'}
                    </Typography.Text>
                  </div>
                )}
                {company && (
                  <div className="company-result">
                    <span className="hint">Компания найдена · Демо-данные</span>
                    <br />
                    <strong>{company.short_name}</strong>
                    <Typography.Text tag="p" view="primary-small">
                      {company.address}
                    </Typography.Text>
                    <Typography.Text tag="p" view="primary-small">
                      Основная деятельность: {company.okved_main.name}
                    </Typography.Text>
                    <Typography.Text tag="p" view="primary-small">
                      Данные реестра доступны только для чтения.
                    </Typography.Text>
                  </div>
                )}
                {company?.inn === inn && known && <PreliminaryEstimate key={inn} inn={inn} />}
                <Typography.Title tag="h2" view="small" font="system" defaultMargins>
                  2. Финансирование
                </Typography.Title>
                <NeedFromText
                  disabled={busy || recommendBusy}
                  onNeed={(need) => {
                    setGoal(need.goal);
                    if (need.amount_kop) setAmount(String(need.amount_kop / 100));
                    if (need.term_months) setTerm(String(need.term_months));
                    invalidateRecommendation();
                  }}
                />
                <Select
                  block
                  label="На что нужны деньги"
                  options={options(goals)}
                  selected={goal}
                  disabled={busy || recommendBusy}
                  onChange={({ selected }) => {
                    setGoal(selected!.key as components['schemas']['Goal']);
                    invalidateRecommendation();
                  }}
                />
                <FieldPair>
                  <MoneyField
                    label="Сумма, ₽"
                    hint={
                      selectedProduct
                        ? `От ${money(selectedProduct.amount_min_kop, true)} до ${money(selectedProduct.amount_max_kop, true)}`
                        : 'Укажите сумму финансирования'
                    }
                    error={
                      amount &&
                      selectedProduct &&
                      (kop(amount) < selectedProduct.amount_min_kop || kop(amount) > selectedProduct.amount_max_kop)
                        ? 'Сумма вне диапазона выбранного продукта'
                        : undefined
                    }
                    value={amount}
                    onChange={(value) => {
                      invalidateRecommendation();
                      setAmount(value);
                    }}
                    required
                    disabled={busy}
                  />
                  <Input
                    block
                    label="Срок, месяцев"
                    hint={
                      selectedProduct
                        ? `От ${selectedProduct.term_min_months} до ${selectedProduct.term_max_months} месяцев`
                        : undefined
                    }
                    type="number"
                    min={1}
                    max={60}
                    value={term}
                    onChange={(event) => {
                      invalidateRecommendation();
                      setTerm(event.target.value);
                    }}
                    required
                    disabled={busy}
                  />
                </FieldPair>
                <MoneyField
                  label="Среднемесячная выручка, ₽"
                  value={revenue}
                  onChange={(value) => {
                    invalidateRecommendation();
                    setRevenue(value);
                  }}
                  disabled={busy}
                />
                <Button
                  type="button"
                  loading={recommendBusy}
                  disabled={busy || Number(amount) <= 0 || !amount || Number(term) < 1 || Number(term) > 60}
                  onClick={recommend}
                >
                  Подобрать продукт
                </Button>
                {recommendation && (
                  <ProductChoices
                    items={recommendation.items}
                    selected={chosenProduct}
                    disabled={busy || recommendBusy}
                    onSelect={(item) => {
                      key.current = null;
                      setChosenProduct(item.product);
                      setAmount(String(item.amount_kop / 100));
                      setTerm(String(item.term_months));
                    }}
                  />
                )}
                {catalogError && <InlineNotice>{catalogError}</InlineNotice>}
                <Select
                  block
                  label="Выбранный продукт"
                  options={products.map((item) => ({ key: item.code, content: item.name }))}
                  selected={chosenProduct}
                  disabled={busy || !products.length}
                  onChange={({ selected }) => {
                    key.current = null;
                    setChosenProduct(selected!.key as components['schemas']['ProductCode']);
                    const item = recommendation?.items.find((item) => item.product === selected?.key);
                    if (item) {
                      setAmount(String(item.amount_kop / 100));
                      setTerm(String(item.term_months));
                    }
                  }}
                />
                {selectedProduct && (
                  <div className="product-terms">
                    <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
                      Условия продукта
                    </Typography.Title>
                    <Typography.Text tag="p" view="primary-small">
                      {!recommendation
                        ? ''
                        : recommendation.items.some((item) => item.product === chosenProduct)
                          ? 'Подходит для выбранной цели. '
                          : 'Для этой цели банк рекомендует другой продукт, он указан выше. '}
                      Окончательные условия банк определит после рассмотрения заявки.
                    </Typography.Text>
                    <Typography.Text tag="p" view="primary-small">
                      Сумма кредита: от {money(selectedProduct.amount_min_kop, true)} до{' '}
                      {money(selectedProduct.amount_max_kop, true)}. Срок: от {selectedProduct.term_min_months} до{' '}
                      {selectedProduct.term_max_months} месяцев.
                    </Typography.Text>
                    <Typography.Text tag="p" view="primary-small">
                      Ставка: от {rate(selectedProduct.rate_min_bp)} до {rate(selectedProduct.rate_max_bp)} годовых.
                      Возраст бизнеса: от {selectedProduct.min_business_age_months} мес.
                    </Typography.Text>
                    <ul>
                      {selectedProduct.requirements.map((requirement) => (
                        <li key={requirement}>{requirement}</li>
                      ))}
                    </ul>
                    <Typography.Text tag="p" view="primary-small">
                      {selectedProduct.repayment === 'auto_debit'
                        ? 'Задолженность погашается автоматически из поступлений на счёт.'
                        : 'Погашение равными ежемесячными платежами.'}
                    </Typography.Text>
                  </div>
                )}
                {chosenProduct === 'equipment' && (
                  <MoneyField
                    label="Стоимость оборудования, ₽"
                    hint="Кредит может покрыть до 90% стоимости оборудования"
                    required
                    value={equipmentCost}
                    onChange={setEquipmentCost}
                    disabled={busy}
                  />
                )}
                <Typography.Title tag="h2" view="small" font="system" defaultMargins>
                  3. Представитель и согласия
                </Typography.Title>
                <Typography.Text tag="p" view="primary-small" className="hint">
                  Если вы не руководитель компании и не сам ИП, выберите работу по доверенности. Файл можно приложить
                  после сохранения черновика, до отправки в банк.
                </Typography.Text>
                <Checkbox
                  label="Действую по доверенности"
                  checked={proxy}
                  onChange={(_, payload) => {
                    key.current = null;
                    setProxy(payload.checked);
                  }}
                  disabled={busy}
                />
                <Checkbox
                  label="Согласен на обработку персональных данных для рассмотрения заявки"
                  checked={personalConsent}
                  onChange={(_, payload) => setPersonalConsent(payload.checked)}
                  disabled={busy}
                />
                <Checkbox
                  label="Согласен на запрос кредитной истории в бюро кредитных историй"
                  checked={consent}
                  onChange={(_, payload) => {
                    key.current = null;
                    setConsent(payload.checked);
                  }}
                  disabled={busy}
                />
                <Button
                  type="submit"
                  view="primary"
                  loading={busy}
                  disabled={!found || !selectedProduct || !personalConsent}
                >
                  Сохранить черновик
                </Button>
              </form>
            )}
          </>
        )}
      </section>
    </div>
  );
}

// подать можно и без документов, но тогда сразу дозапрос с коротким сроком: предупреждаем до кнопки
function submitWarning(card: App, product?: components['schemas']['Product']) {
  const missing = requiredDocs(card, product).filter((kind) => !card.documents.some((doc) => doc.kind === kind));
  const unconfirmed = card.documents.some(
    (doc) => ['statement', 'financials'].includes(doc.kind) && doc.status !== 'confirmed',
  );
  const consents = [
    !card.pd_consent && 'обработку персональных данных',
    !card.bki_consent && 'запрос кредитной истории',
  ].filter(Boolean);
  return [
    consents.length && `Нет согласия на ${consents.join(' и ')}, отметьте его в разделе «Изменить данные заявки».`,
    missing.length &&
      `Не приложено: ${missing.map((kind) => kinds[kind].toLowerCase()).join(', ')}. Банк запросит это после отправки.`,
    unconfirmed && 'Итоги выписки или отчётности не подтверждены, в расчёт они не попадут.',
  ]
    .filter(Boolean)
    .join(' ');
}

const draftDetails = (card: App) => ({
  product: card.product,
  purpose: card.purpose,
  representative_role: card.representative_role,
  monthly_revenue_kop: card.monthly_revenue_kop ? String(card.monthly_revenue_kop / 100) : '',
  equipment_cost_kop: card.equipment_cost_kop ? String(card.equipment_cost_kop / 100) : '',
  comment: card.comment ?? '',
});

function DraftEditor({ card, onSaved }: { card: App; onSaved: (card: App) => void }) {
  const [snapshot, setSnapshot] = useState(card);
  const [amount, setAmount] = useState(String(card.amount_kop / 100));
  const [term, setTerm] = useState(String(card.term_months));
  const [consent, setConsent] = useState(card.bki_consent);
  const [personalConsent, setPersonalConsent] = useState(card.pd_consent);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [details, setDetails] = useState(() => draftDetails(card));
  const [conflict, setConflict] = useState(false);
  const stale = conflict || snapshot.version !== card.version;
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || stale) return;
    setBusy(true);
    setMessage('');
    try {
      const { data, response, error } = await api.PATCH('/applications/{id}', {
        params: { path: { id: card.id } },
        body: {
          version: snapshot.version,
          amount_kop: kop(amount),
          term_months: Number(term),
          bki_consent: consent,
          pd_consent: personalConsent,
          product: details.product,
          purpose: details.purpose,
          representative_role: details.representative_role,
          comment: details.comment,
          ...(details.monthly_revenue_kop ? { monthly_revenue_kop: kop(details.monthly_revenue_kop) } : {}),
          ...(details.product === 'equipment' && details.equipment_cost_kop
            ? { equipment_cost_kop: kop(details.equipment_cost_kop) }
            : {}),
        },
      });
      if (data) {
        setSnapshot(data);
        onSaved(data);
        setMessage('');
        notify('Изменения сохранены.');
      } else {
        if (response.status === 409) setConflict(true);
        setMessage(response.status === 409 ? '' : errorText(error, 'Не удалось сохранить изменения.'));
      }
    } catch {
      setMessage('Нет связи с сервером. Проверьте карточку перед повторным сохранением.');
    } finally {
      setBusy(false);
    }
  }
  async function reset() {
    setBusy(true);
    try {
      const { data } = await api.GET('/applications/{id}', { params: { path: { id: card.id } } });
      if (!data) {
        setMessage('Не удалось обновить данные. Ваш ввод сохранён.');
        return;
      }
      setSnapshot(data);
      onSaved(data);
      setAmount(String(data.amount_kop / 100));
      setTerm(String(data.term_months));
      setConsent(data.bki_consent);
      setPersonalConsent(data.pd_consent);
      setDetails(draftDetails(data));
      setConflict(false);
      setMessage('');
    } catch {
      setMessage('Нет связи с сервером. Ваш ввод сохранён.');
    } finally {
      setBusy(false);
    }
  }
  return (
    <form onSubmit={save}>
      <Typography.Title tag="h3" view="xsmall" font="system" defaultMargins>
        Изменить черновик
      </Typography.Title>
      <Select
        block
        label="Выбранный продукт"
        selected={details.product}
        options={options(productNames)}
        disabled={busy}
        onChange={({ selected }) => setDetails({ ...details, product: selected!.key as App['product'] })}
      />
      <Select
        block
        label="На что нужны деньги"
        selected={details.purpose}
        options={options(goals)}
        disabled={busy}
        onChange={({ selected }) => setDetails({ ...details, purpose: selected!.key as App['purpose'] })}
      />
      <Select
        block
        label="Полномочия представителя"
        selected={details.representative_role}
        options={options(representatives)}
        disabled={busy}
        onChange={({ selected }) =>
          setDetails({ ...details, representative_role: selected!.key as App['representative_role'] })
        }
      />
      <MoneyField
        label="Среднемесячная выручка, ₽"
        value={details.monthly_revenue_kop}
        disabled={busy}
        onChange={(value) => setDetails({ ...details, monthly_revenue_kop: value })}
      />
      {details.product === 'equipment' && (
        <MoneyField
          label="Стоимость оборудования, ₽"
          value={details.equipment_cost_kop}
          disabled={busy}
          onChange={(value) => setDetails({ ...details, equipment_cost_kop: value })}
        />
      )}
      <Input
        block
        label="Комментарий к заявке"
        maxLength={1000}
        value={details.comment}
        disabled={busy}
        onChange={(event) => setDetails({ ...details, comment: event.target.value })}
      />
      {stale && (
        <InlineNotice>
          Заявка изменилась. Ваш ввод сохранён в форме. «Обновить данные» заменит его актуальными значениями.
        </InlineNotice>
      )}
      <MoneyField label="Сумма, ₽" required value={amount} onChange={setAmount} disabled={busy} />
      <Input
        block
        label="Срок, месяцев"
        type="number"
        min={1}
        max={60}
        required
        value={term}
        onChange={(event) => setTerm(event.target.value)}
        disabled={busy}
      />
      <Checkbox
        label="Согласие на обработку персональных данных"
        checked={personalConsent}
        onChange={(_, payload) => setPersonalConsent(payload.checked)}
        disabled={busy}
      />
      <Checkbox
        label="Согласие на запрос кредитной истории"
        checked={consent}
        onChange={(_, payload) => setConsent(payload.checked)}
        disabled={busy}
      />
      {message && (
        <Typography.Text tag="p" view="primary-small" role="status">
          {message}
        </Typography.Text>
      )}
      <Button type="submit" loading={busy} disabled={stale}>
        Сохранить изменения
      </Button>
      {stale && (
        <Button type="button" disabled={busy} onClick={reset}>
          Обновить данные
        </Button>
      )}
    </form>
  );
}
