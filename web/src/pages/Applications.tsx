import {
  goals,
  kinds,
  kop,
  labels,
  money,
  rate,
  options,
  products as productNames,
  representatives,
  requiredDocs,
} from '../api/presentation';
import { Icon, PageHeading, StatusBadge, FieldPair } from '../ui/Workspace';
import { useCabinetCache } from '../CabinetCache';
import { applicationGroups, isApplicationGroup, isCompleted } from '../api/dashboard';
import { CompletedJourney } from './CompletedJourney';
import { Value } from '../motion/Value';
import {
  motion,
  AnimatePresence,
  Button,
  Checkbox,
  ListItem,
  MotionDetails,
  Select,
  StatusText,
  useMotionDisabled,
} from '../motion';
import { BottomNotice, notify } from '../BottomNotice';
import { Progress } from './Progress';
import { BankDetails } from './BankDetails';
import { Decision } from './Decision';
import { Documents } from './Documents';
import { useEffect, useState, useRef, type FormEvent } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router';
import { Input } from '@alfalab/core-components/input';
import { api, errorText, settle } from '../api/client';
import type { components, operations } from '../api/schema.gen';

type App = components['schemas']['ApplicationView'];
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

export function Applications({
  bank = false,
  createOnly = false,
  role,
}: {
  bank?: boolean;
  createOnly?: boolean;
  role?: components['schemas']['Role'];
}) {
  const reduced = useMotionDisabled();
  const cache = useCabinetCache();
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
  const [connectionFailed, setConnectionFailed] = useState(false);
  const [revision, setRevision] = useState(0);
  const [status, setStatus] = useState<components['schemas']['ApplicationStatus'] | undefined>();
  const [offset, setOffset] = useState(0);
  const [total, setTotal] = useState(cached?.total ?? 0);
  const [product, setProduct] = useState<components['schemas']['ProductCode'] | undefined>();
  const [grade, setGrade] = useState<components['schemas']['Grade'] | undefined>();
  const [sort, setSort] = useState<Sort>('-created_at');
  const [inn, setInn] = useState('');
  const [amount, setAmount] = useState('');
  const [term, setTerm] = useState('24');
  const [consent, setConsent] = useState(false);
  const [personalConsent, setPersonalConsent] = useState(false);
  const [proxy, setProxy] = useState(false);
  const [company, setCompany] = useState<components['schemas']['Company'] | null>(null);
  const [lookupBusy, setLookupBusy] = useState(false);
  const [chosenProduct, setChosenProduct] = useState<components['schemas']['ProductCode']>('working_capital');
  const [goal, setGoal] = useState<components['schemas']['Goal']>('purchase');
  const [revenue, setRevenue] = useState('');
  const [equipmentCost, setEquipmentCost] = useState('');
  const [recommendation, setRecommendation] = useState<components['schemas']['Recommendation'] | null>(null);
  const [recommendBusy, setRecommendBusy] = useState(false);
  const [products, setProducts] = useState<components['schemas']['Product'][]>(cache.get('products') ?? []);
  const [catalogError, setCatalogError] = useState('');
  const selectedProduct = products.find((item) => item.code === chosenProduct);
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
  const lookupVersion = useRef(0);
  const recommendationVersion = useRef(0);
  const key = useRef<string | null>(null);
  const loaded = useRef(!id && !!cached);
  async function lookup() {
    const version = ++lookupVersion.current;
    setLookupBusy(true);
    setError('');
    setCompany(null);
    try {
      const { data, error } = await api.GET('/companies/{inn}', { params: { path: { inn } } });
      if (version !== lookupVersion.current) return;
      if (data) setCompany(data);
      else setError(errorText(error, 'Не удалось найти компанию.'));
    } catch {
      if (version === lookupVersion.current) setError('Нет связи с реестром. Попробуйте ещё раз.');
    } finally {
      if (version === lookupVersion.current) setLookupBusy(false);
    }
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
      if (data) setRecommendation(data);
      else setError(errorText(error, 'Не удалось подобрать продукт.'));
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
    let pending = false;
    async function load() {
      const request = ++serial;
      pending = true;
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
          pending = false;
          setLoading(false);
        }
      }
    }
    window.addEventListener('application-refresh', load);
    void load();
    const events = new EventSource('/api/events');
    events.onopen = () => {
      setConnectionFailed(false);
      if (!pending) void load();
      window.dispatchEvent(new Event('feed-refresh'));
    };
    events.onerror = () => {
      setConnectionFailed(true);
      void api.GET('/me').catch(() => undefined);
    };
    events.addEventListener('status', load);
    const updateFeed = () => window.dispatchEvent(new Event('feed-refresh'));
    events.addEventListener('status', updateFeed);
    events.addEventListener('document', load);
    return () => {
      active = false;
      events.close();
      window.removeEventListener('application-refresh', load);
    };
  }, [id, revision, offset, status, product, grade, sort, bank, cache, createOnly, group]);
  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (busy || !company || company.inn !== inn || !selectedProduct) return;
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
          representative_role: proxy ? 'proxy' : company.kind === 'IP' ? 'owner_ip' : 'director',
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
        notify('Заявка отправлена в банк.');
      } else setError(errorText(problem, 'Не удалось отправить заявку.'));
    } catch {
      setError('Нет связи с сервером. Проверьте статус перед повторной отправкой.');
    } finally {
      setBusy(false);
    }
  }
  const [title, description] = bank
    ? ['Очередь банка', 'Заявки клиентов, проверки и решения']
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
        {loading && !createOnly && (
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
                    <Progress card={card} />
                    <h2>Заявка № {card.number}</h2>
                    <p>
                      {card.company.short_name}, ИНН {card.company.inn}
                    </p>
                    <p>
                      <StatusText value={card.status}>{labels[card.status]}</StatusText>
                    </p>
                    <p>
                      {money(card.amount_kop)}, {card.term_months} мес.
                    </p>
                    {!bank && (
                      <p>
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
                            signing: 'Подпишите документы кодом ниже.',
                            rejected: 'Банк не одобрил заявку. Причины указаны ниже.',
                            issued: 'Выдача завершена.',
                            closed: 'Заявка завершена.',
                            cancelled_by_client: 'Заявка отменена.',
                            expired: 'Срок действия заявки истёк.',
                          }[card.status]
                        }
                      </p>
                    )}
                    {!bank && card.status === 'draft' && (
                      <MotionDetails className="edit-panel">
                        <summary>Изменить данные заявки</summary>
                        <DraftEditor
                          key={card.id}
                          card={card}
                          onSaved={(data) => {
                            cache.rememberApplication(data);
                            setCard(data);
                          }}
                        />
                      </MotionDetails>
                    )}
                    {!bank && card.allowed_next.includes('submitted') && (
                      <>
                        {warning && <p role="note">{warning}</p>}
                        <Button view="primary" loading={busy} onClick={submit}>
                          Отправить в банк
                        </Button>
                      </>
                    )}
                    {!bank && <Decision key={card.id} card={card} refresh={() => setRevision((value) => value + 1)} />}
                    {bank && <BankDetails key={`bank-${card.id}`} card={card} role={role} />}
                    <Documents
                      key={`documents-${card.id}`}
                      card={card}
                      bank={bank}
                      product={cardProduct}
                      refresh={() => setRevision((value) => value + 1)}
                    />
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
                      ...Object.entries(applicationGroups).map(([key, value]) => ({
                        key: `group:${key}`,
                        content: value.label,
                      })),
                      ...options(labels),
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
                    options={sorts}
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
                      <p>
                        {status || product || grade || group ? 'По выбранным условиям заявок нет.' : 'Заявок пока нет.'}
                      </p>
                    )}
                    <motion.ul layout={reduced ? false : 'position'} className="applications-list">
                      <AnimatePresence initial={false}>
                        {items.map((item) => (
                          <ListItem className="application-card" key={item.id}>
                            <Link to={`${root}/applications/${item.id}`}>
                              № {item.number}: {item.company.short_name}
                            </Link>
                            <StatusBadge status={item.status} />
                            <strong className="application-amount">
                              <Value text={money(item.amount_kop)} />
                            </strong>
                            <span>{item.term_months} мес.</span>
                          </ListItem>
                        ))}
                      </AnimatePresence>
                    </motion.ul>
                    <p>
                      Всего заявок: {total}. Страница {Math.floor(offset / 20) + 1}
                    </p>
                    <Button size={40} disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 20))}>
                      Назад
                    </Button>
                    <Button size={40} disabled={offset + 20 >= total} onClick={() => setOffset(offset + 20)}>
                      Далее
                    </Button>
                  </>
                )}
              </>
            )}
            {!bank && createOnly && (
              <form
                className="application-form"
                onSubmit={create}
                onChange={() => {
                  recommendationVersion.current++;
                  key.current = null;
                  setRecommendation(null);
                }}
              >
                <h2>1. Компания</h2>
                <p className="hint">Найдём сведения по ИНН. Затем укажите цель и параметры кредита.</p>
                <Input
                  block
                  label="ИНН компании"
                  value={inn}
                  onChange={(event) => {
                    lookupVersion.current++;
                    setLookupBusy(false);
                    setCompany(null);
                    key.current = null;
                    setInn(event.target.value);
                  }}
                  required
                  pattern="[0-9]{10}([0-9]{2})?"
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
                {company && (
                  <div className="company-result">
                    <span className="hint">Компания найдена · Демо-данные</span>
                    <br />
                    <strong>{company.short_name}</strong>
                    <p>{company.address}</p>
                    <p>Основная деятельность: {company.okved_main.name}</p>
                    <p>Данные реестра доступны только для чтения.</p>
                  </div>
                )}
                <h2>2. Финансирование</h2>
                <Select
                  block
                  label="На что нужны деньги"
                  options={options(goals)}
                  selected={goal}
                  disabled={busy || recommendBusy}
                  onChange={({ selected }) => {
                    setGoal(selected!.key as components['schemas']['Goal']);
                    key.current = null;
                    setRecommendation(null);
                  }}
                />
                <FieldPair>
                  <Input
                    block
                    label="Сумма, ₽"
                    type="number"
                    min={1}
                    step="0.01"
                    value={amount}
                    onChange={(event) => {
                      key.current = null;
                      setAmount(event.target.value);
                    }}
                    required
                    disabled={busy}
                  />
                  <Input
                    block
                    label="Срок, месяцев"
                    type="number"
                    min={1}
                    max={60}
                    value={term}
                    onChange={(event) => {
                      key.current = null;
                      setTerm(event.target.value);
                    }}
                    required
                    disabled={busy}
                  />
                </FieldPair>
                <Input
                  block
                  label="Среднемесячная выручка, ₽"
                  type="number"
                  min={1}
                  step="0.01"
                  value={revenue}
                  onChange={(event) => setRevenue(event.target.value)}
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
                  <div>
                    <p>Предварительный расчёт, окончательные условия определит банк.</p>
                    {recommendation.items.map((item, index) => (
                      <p key={`${item.product}-${index}`}>
                        {item.primary ? 'Рекомендация' : 'Альтернатива'}: {productNames[item.product]},{' '}
                        {money(item.amount_kop)}, {item.term_months} мес., {rate(item.rate_bp)} годовых. Ориентир
                        платежа: {money(item.monthly_payment_kop)}.
                      </p>
                    ))}
                  </div>
                )}
                {catalogError && <p role="alert">{catalogError}</p>}
                <Select
                  block
                  label="Выбранный продукт"
                  options={products.map((item) => ({ key: item.code, content: item.name }))}
                  selected={chosenProduct}
                  disabled={busy || !products.length}
                  onChange={({ selected }) => {
                    key.current = null;
                    setChosenProduct(selected!.key as components['schemas']['ProductCode']);
                  }}
                />
                {selectedProduct && (
                  <div className="product-terms">
                    <h3>Условия продукта</h3>
                    <p>Подходит для выбранной цели. Окончательные условия банк определит после рассмотрения заявки.</p>
                    <p>
                      Сумма кредита: от {money(selectedProduct.amount_min_kop)} до{' '}
                      {money(selectedProduct.amount_max_kop)}. Срок: от {selectedProduct.term_min_months} до{' '}
                      {selectedProduct.term_max_months} месяцев.
                    </p>
                    <p>
                      Ставка: от {selectedProduct.rate_min_bp / 100}% до {selectedProduct.rate_max_bp / 100}% годовых.
                      Возраст бизнеса: от {selectedProduct.min_business_age_months} мес.
                    </p>
                    <ul>
                      {selectedProduct.requirements.map((requirement) => (
                        <li key={requirement}>{requirement}</li>
                      ))}
                    </ul>
                    <p>
                      {selectedProduct.repayment === 'auto_debit'
                        ? 'Задолженность погашается автоматически из поступлений на счёт.'
                        : 'Погашение равными ежемесячными платежами.'}
                    </p>
                  </div>
                )}
                {chosenProduct === 'equipment' && (
                  <Input
                    block
                    label="Стоимость оборудования, ₽"
                    type="number"
                    min={1}
                    step="0.01"
                    required
                    value={equipmentCost}
                    onChange={(event) => setEquipmentCost(event.target.value)}
                    disabled={busy}
                  />
                )}
                <h2>3. Представитель и согласия</h2>
                <p className="hint">
                  Если вы не руководитель компании и не сам ИП, выберите работу по доверенности. Файл можно приложить
                  после сохранения черновика, до отправки в банк.
                </p>
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
                  disabled={!company || company.inn !== inn || !selectedProduct || !personalConsent}
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
  return [
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
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [details, setDetails] = useState(() => draftDetails(card));
  const stale = snapshot.version !== card.version;
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
          product: details.product,
          purpose: details.purpose,
          representative_role: details.representative_role,
          comment: details.comment,
          ...(details.monthly_revenue_kop ? { monthly_revenue_kop: kop(details.monthly_revenue_kop) } : {}),
          ...(details.equipment_cost_kop ? { equipment_cost_kop: kop(details.equipment_cost_kop) } : {}),
        },
      });
      if (data) {
        setSnapshot(data);
        onSaved(data);
        setMessage('');
        notify('Изменения сохранены.');
      } else
        setMessage(
          response.status === 409
            ? 'Данные изменились. Обновите карточку и проверьте поля перед повторным сохранением.'
            : errorText(error, 'Не удалось сохранить изменения.'),
        );
    } catch {
      setMessage('Нет связи с сервером. Проверьте карточку перед повторным сохранением.');
    } finally {
      setBusy(false);
    }
  }
  function reset() {
    setSnapshot(card);
    setAmount(String(card.amount_kop / 100));
    setTerm(String(card.term_months));
    setConsent(card.bki_consent);
    setDetails(draftDetails(card));
    setMessage('');
  }
  return (
    <form onSubmit={save}>
      <h3>Изменить черновик</h3>
      <Select
        block
        label="Продукт черновика"
        selected={details.product}
        options={options(productNames)}
        disabled={busy}
        onChange={({ selected }) => setDetails({ ...details, product: selected!.key as App['product'] })}
      />
      <Select
        block
        label="Цель черновика"
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
      <Input
        block
        label="Выручка черновика за месяц, ₽"
        type="number"
        min={1}
        step="0.01"
        value={details.monthly_revenue_kop}
        disabled={busy}
        onChange={(event) => setDetails({ ...details, monthly_revenue_kop: event.target.value })}
      />
      <Input
        block
        label="Стоимость оборудования в черновике, ₽"
        type="number"
        min={1}
        step="0.01"
        value={details.equipment_cost_kop}
        disabled={busy}
        onChange={(event) => setDetails({ ...details, equipment_cost_kop: event.target.value })}
      />
      <Input
        block
        label="Комментарий к заявке"
        maxLength={1000}
        value={details.comment}
        disabled={busy}
        onChange={(event) => setDetails({ ...details, comment: event.target.value })}
      />
      {stale && (
        <p role="alert">
          Заявка изменилась. Ваш ввод сохранён в форме. Загрузите актуальные значения перед сохранением.
        </p>
      )}
      <Input
        block
        label="Сумма, ₽"
        type="number"
        min={1}
        step="0.01"
        required
        value={amount}
        onChange={(event) => setAmount(event.target.value)}
        disabled={busy}
      />
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
        label="Согласие на запрос кредитной истории"
        checked={consent}
        onChange={(_, payload) => setConsent(payload.checked)}
        disabled={busy}
      />
      {message && <p role="status">{message}</p>}
      <Button type="submit" loading={busy} disabled={stale}>
        Сохранить изменения
      </Button>
      {stale && (
        <Button type="button" disabled={busy} onClick={reset}>
          Заменить мой ввод актуальными данными
        </Button>
      )}
    </form>
  );
}
