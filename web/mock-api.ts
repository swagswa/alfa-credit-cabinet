import { validDate } from './src/api/date';
import { readFileSync } from 'node:fs';
import { parse } from 'yaml';
import type { Plugin } from 'vite';
import { randomUUID, randomInt, createHash } from 'node:crypto';
import type { components } from './src/api/schema.gen';
import type { ServerResponse } from 'node:http';

type App = components['schemas']['ApplicationView'];

type Schema = {
  $ref?: string;
  example?: unknown;
  allOf?: Schema[];
  properties?: Record<string, Schema>;
  items?: Schema;
  enum?: unknown[];
  type?: string;
  default?: unknown;
  minimum?: number;
};

type ResponseSpec = { content?: Record<string, { schema: Schema; example?: unknown }> };
type Operation = { responses: Record<string, ResponseSpec> };

// Данные в памяти процесса, кредитные решения не моделируем.
export function mockApi(): Plugin {
  const companyNames: Record<string, string> = {
    '0000100012': 'ООО «Тёплое зерно»',
    '000020001010': 'ИП Лебедев С. Н.',
    '0000300011': 'ООО «Кодовая база»',
    '0000400016': 'ООО «Стройснаб»',
    '0000500010': 'ООО «Северный путь»',
  };
  const feedCreatedAt = new Date().toISOString();
  const sessions = new Map<string, components['schemas']['Me']>();
  const applications = new Map<string, { owner: string; app: App }>();
  const keys = new Map<string, { body: string; id: string }>();
  const documents = new Map<
    string,
    { owner: string; document: components['schemas']['Document']; bytes: Uint8Array }
  >();
  const signing = new Map<string, components['schemas']['SigningCode'] & { code: string }>();
  // уведомления по логину клиента; в моке только код подписи
  const notices = new Map<string, components['schemas']['Notification'][]>();
  const streams = new Set<{ login: string; role: string; session: string; id: string | null; res: ServerResponse }>();
  const histories = new Map<string, components['schemas']['ApplicationEvent'][]>();
  const previousStatus = new Map<string, App['status']>();
  function publish(owner: string, app: App, role: components['schemas']['ActorRole'] = 'system', comment = '') {
    const history = histories.get(app.id) ?? [];
    const from = previousStatus.get(app.id);
    if (from !== app.status) {
      history.push({
        id: history.length + 1,
        kind: 'transition',
        from_status: from,
        to_status: app.status,
        actor_role: role,
        comment,
        reason_codes: app.reasons.map((r) => r.code),
        created_at: new Date().toISOString(),
      });
      histories.set(app.id, history);
      previousStatus.set(app.id, app.status);
    }
    for (const stream of streams) {
      if (!sessions.has(stream.session)) {
        stream.res.end();
        streams.delete(stream);
        continue;
      }
      if ((stream.role !== 'client' || stream.login === owner) && (!stream.id || stream.id === app.id)) {
        stream.res.write(
          `event: status\ndata: ${JSON.stringify({ application_id: app.id, version: app.version, status: app.status })}\n\n`,
        );
      }
    }
  }
  const spec = parse(readFileSync(new URL('../api/openapi.yaml', import.meta.url), 'utf8'));
  const productRules = parse(readFileSync(new URL('../rules/products.yaml', import.meta.url), 'utf8'));
  function sample(schema: Schema): unknown {
    if (schema.example !== undefined) return schema.example;
    if (schema.$ref) return sample(spec.components.schemas[schema.$ref.split('/').pop()!]);
    if (schema.allOf) return Object.assign({}, ...schema.allOf.map(sample));
    if (schema.properties)
      return Object.fromEntries(Object.entries(schema.properties).map(([key, value]) => [key, sample(value)]));
    if (schema.items) return [sample(schema.items)];
    return (
      schema.default ??
      schema.enum?.[0] ??
      (schema.type === 'boolean'
        ? false
        : schema.type === 'integer' || schema.type === 'number'
          ? (schema.minimum ?? 0)
          : '')
    );
  }
  const routes = Object.entries(spec.paths as Record<string, Record<string, Operation>>).map(([path, operations]) => ({
    pattern: new RegExp(`^/api${path.replace(/\{[^}]+\}/g, '[^/]+')}$`),
    operations,
  }));
  return {
    name: 'contract-mock',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        const pathname = new URL(req.url ?? '/', 'http://localhost').pathname;
        if (!pathname.startsWith('/api/')) return next();
        const token = req.headers.cookie
          ?.split(';')
          .map((part) => part.trim())
          .find((part) => part.startsWith('mock_session='))
          ?.slice(13);
        const actor = token ? sessions.get(token) : undefined;
        function json(status: number, body: unknown) {
          res.statusCode = status;
          res.setHeader('Content-Type', status >= 400 ? 'application/problem+json' : 'application/json');
          res.end(JSON.stringify(body));
        }
        function problem(status: number, title: string) {
          json(status, { type: 'about:blank', title, status });
        }
        if (pathname === '/api/login' && req.method === 'POST') {
          try {
            let body = '';
            for await (const chunk of req) {
              body += chunk;
              if (body.length > 4096) return problem(413, 'Слишком большой запрос');
            }
            const { login, password } = JSON.parse(body);
            const clients = ['coffee', 'logistics', 'it', 'build', 'unavailable'];
            const staff = ['manager', 'underwriter', 'admin'] as const;
            if (password !== 'demo12345' || ![...clients, ...staff].includes(login))
              return problem(401, 'Неверный логин или пароль');
            const role = staff.find((value) => value === login) ?? 'client';
            const me = sample(spec.components.schemas.Me) as components['schemas']['Me'];
            me.user = { ...me.user, login, role, full_name: login };
            if (role !== 'client') me.companies = [];
            const id = randomUUID();
            sessions.set(id, me);
            res.setHeader('Set-Cookie', `mock_session=${id}; Path=/; HttpOnly; SameSite=Lax`);
            return json(200, me);
          } catch {
            return problem(400, 'Некорректный запрос');
          }
        }
        if (pathname === '/api/me' && req.method === 'GET') {
          if (actor) return json(200, actor);
          res.statusCode = 204;
          return res.end();
        }
        if (pathname !== '/api/health' && !actor) return problem(401, 'Войдите в кабинет');
        if (pathname === '/api/logout' && req.method === 'POST') {
          if (token) {
            sessions.delete(token);
            for (const stream of streams)
              if (stream.session === token) {
                stream.res.end();
                streams.delete(stream);
              }
          }
          res.setHeader('Set-Cookie', 'mock_session=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0');
          res.statusCode = 204;
          return res.end();
        }
        if (pathname.endsWith('/preliminary') && req.method === 'GET' && actor) {
          const inn = pathname.split('/')[3];
          if (actor.user.role !== 'client') return problem(403, 'Ориентир доступен клиенту');
          if (!actor.companies.some((c) => c.company.inn === inn)) return problem(404, 'Компания не найдена');
          if (inn !== '0000100012') return problem(503, 'Демо-ориентир доступен для кофейни');
          return json(200, sample(spec.components.schemas.PreliminaryEstimate));
        }
        if (pathname.startsWith('/api/companies/') && req.method === 'GET') {
          const inn = pathname.split('/').pop()!;
          if (!/^\d{10}(\d{2})?$/.test(inn)) return problem(400, 'Проверьте ИНН');
          if (inn === '0000500010') return problem(503, 'Реестр временно недоступен');
          return json(200, {
            ...(sample(spec.components.schemas.Company) as object),
            inn,
            short_name: companyNames[inn] ?? 'Демонстрационная компания',
            kind: inn.length === 12 ? 'IP' : 'UL',
            simulated: true,
          });
        }
        if (pathname.startsWith('/api/samples/') && req.method === 'GET') {
          const inn = pathname.split('/').pop()!;
          if (!/^\d{10}(\d{2})?$/.test(inn)) return problem(400, 'Проверьте ИНН');
          try {
            const bytes = readFileSync(new URL(`../testdata/documents/${inn}_statement_c.png`, import.meta.url));
            res.setHeader('Content-Type', 'image/png');
            return res.end(bytes);
          } catch {
            return problem(404, 'Для этой компании пример не подготовлен');
          }
        }
        if (pathname === '/api/products' && req.method === 'GET') {
          const products = Object.entries(
            productRules.products as Record<
              string,
              {
                name: string;
                description: string;
                amount_min_rub: number;
                amount_max_rub: number;
                term_min_months: number;
                term_max_months: number;
                base_rate_pct: number;
                max_rate_pct: number;
                repayment: string;
                min_business_age_months: number;
                requirements: string[];
                documents: {
                  kind: components['schemas']['DocumentKind'];
                  when?: 'no_turnover' | 'amount_over';
                  amount_over_rub?: number;
                }[];
              }
            >,
          ).map(([code, rule]) => ({
            code,
            name: rule.name,
            description: rule.description,
            amount_min_kop: rule.amount_min_rub * 100,
            amount_max_kop: rule.amount_max_rub * 100,
            term_min_months: rule.term_min_months,
            term_max_months: rule.term_max_months,
            rate_min_bp: Math.round(rule.base_rate_pct * 100),
            rate_max_bp: Math.round(rule.max_rate_pct * 100),
            repayment: rule.repayment,
            min_business_age_months: rule.min_business_age_months,
            requirements: rule.requirements,
            documents: rule.documents.map(({ kind, when, amount_over_rub }) => ({
              kind,
              ...(when ? { when } : {}),
              ...(amount_over_rub !== undefined ? { amount_over_kop: amount_over_rub * 100 } : {}),
            })),
          }));
          return json(200, products);
        }
        if (actor && pathname.startsWith('/api/documents/')) {
          const parts = pathname.split('/');
          const entry = documents.get(parts[3]);
          if (!entry || (actor.user.role === 'client' && entry.owner !== actor.user.login))
            return problem(404, 'Документ не найден');
          if (req.method === 'GET' && parts[4] === 'file') {
            res.setHeader('Content-Type', entry.document.content_type);
            res.setHeader(
              'Content-Disposition',
              `attachment; filename*=UTF-8''${encodeURIComponent(entry.document.file_name)}`,
            );
            res.setHeader('X-Content-Type-Options', 'nosniff');
            return res.end(entry.bytes);
          }
          if (req.method === 'GET' && !parts[4]) return json(200, entry.document);
          if (req.method === 'POST' && parts[4] === 'confirm') {
            if (actor.user.role !== 'client') return problem(403, 'Подтверждение доступно клиенту');
            if (entry.document.status === 'confirmed') return problem(409, 'Документ уже подтверждён');
            const appEntry = applications.get(entry.document.application_id)!;
            if (!['draft', 'need_info'].includes(appEntry.app.status))
              return problem(409, 'Подтверждение недоступно на текущем этапе');
            try {
              let raw = '';
              for await (const chunk of req) {
                raw += chunk;
                if (raw.length > 16384) return problem(413, 'Слишком большой запрос');
              }
              const body = JSON.parse(raw) as components['schemas']['DocumentValues'];
              const f = body.financials;
              if (entry.document.kind === 'financials') {
                if (
                  entry.document.kind !== 'financials' ||
                  !f ||
                  body.statement ||
                  f.inn !== appEntry.app.company.inn ||
                  ![new Date().getFullYear() - 1, new Date().getFullYear() - 2].includes(f.year) ||
                  ![
                    f.revenue_kop,
                    f.net_profit_kop,
                    f.liabilities_long_kop,
                    f.liabilities_short_kop,
                    f.balance_total_kop,
                  ].every(Number.isSafeInteger) ||
                  f.revenue_kop < 0 ||
                  f.liabilities_long_kop < 0 ||
                  f.liabilities_short_kop < 0 ||
                  Math.abs(f.net_profit_kop) > f.revenue_kop ||
                  f.balance_total_kop < f.liabilities_long_kop + f.liabilities_short_kop
                )
                  return problem(422, 'Проверьте значения отчётности');
              } else {
                const s = body.statement;
                if (
                  entry.document.kind !== 'statement' ||
                  !s ||
                  body.financials ||
                  s.inn !== appEntry.app.company.inn ||
                  !validDate(s.period_from) ||
                  !validDate(s.period_to) ||
                  s.period_from >= s.period_to ||
                  s.period_to > new Date().toISOString().slice(0, 10) ||
                  !Array.isArray(s.monthly_in) ||
                  s.monthly_in.length < 1 ||
                  s.monthly_in.length > 13
                )
                  return problem(422, 'Проверьте период выписки');
                const months = s.monthly_in.map((item) => item.month);
                if (
                  months[0] !== s.period_from.slice(0, 7) ||
                  months.at(-1) !== s.period_to.slice(0, 7) ||
                  new Set(months).size !== months.length ||
                  months.some(
                    (month, i) =>
                      !/^\d{4}-(0[1-9]|1[0-2])$/.test(month) ||
                      (i > 0 &&
                        new Date(month + '-01T00:00:00Z').getTime() !==
                          Date.UTC(Number(months[i - 1].slice(0, 4)), Number(months[i - 1].slice(5, 7)), 1)),
                  )
                )
                  return problem(422, 'Проверьте последовательность месяцев');
                if (
                  ![
                    s.opening_balance_kop,
                    s.turnover_in_kop,
                    s.turnover_out_kop,
                    s.closing_balance_kop,
                    ...s.monthly_in.map((item) => item.amount_kop),
                  ].every(Number.isSafeInteger) ||
                  s.turnover_in_kop <= 0 ||
                  s.turnover_out_kop < 0 ||
                  s.monthly_in.some((item) => item.amount_kop < 0) ||
                  Math.abs(s.opening_balance_kop + s.turnover_in_kop - s.turnover_out_kop - s.closing_balance_kop) >
                    100 ||
                  Math.abs(s.monthly_in.reduce((sum, item) => sum + item.amount_kop, 0) - s.turnover_in_kop) >
                    months.length * 100
                )
                  return problem(422, 'Суммы выписки не сходятся');
              }
              const extracted = entry.document.extracted?.statement ?? entry.document.extracted?.financials;
              const confirmed = body.statement ?? body.financials;
              entry.document.edited_fields =
                extracted && confirmed
                  ? Object.keys(confirmed).filter(
                      (key) =>
                        JSON.stringify(extracted[key as keyof typeof extracted]) !==
                        JSON.stringify(confirmed[key as keyof typeof confirmed]),
                    )
                  : [];
              entry.document.confirmed = body;
              entry.document.status = 'confirmed';
              entry.document.confirmed_at = new Date().toISOString();
              appEntry.app.documents.find((doc) => doc.id === entry.document.id)!.status = 'confirmed';
              publish(entry.owner, appEntry.app);
              return json(200, entry.document);
            } catch {
              return problem(400, 'Некорректные данные');
            }
          }
          return problem(501, 'Действие документа пока не реализовано');
        }
        if (actor && pathname.startsWith('/api/notifications')) {
          if (actor.user.role !== 'client') return problem(403, 'Уведомления доступны клиенту');
          const list = notices.get(actor.user.login) ?? [];
          if (req.method === 'GET') return json(200, list);
          const at = new Date().toISOString();
          for (const n of list) if (pathname.endsWith('read-all') || pathname.includes(`/${n.id}/`)) n.read_at ??= at;
          res.writeHead(204).end();
          return;
        }
        if (actor && pathname === '/api/feed' && req.method === 'GET') {
          if (actor.user.role !== 'client') return problem(403, 'Лента доступна клиенту');
          const url = new URL(req.url!, 'http://localhost');
          if (
            url.searchParams.has('company_id') &&
            !actor.companies.some((member) => member.company.id === url.searchParams.get('company_id'))
          )
            return problem(404, 'Компания не найдена');
          const logistics = actor.user.login === 'logistics';
          const tips: components['schemas']['FeedItem'][] = [
            {
              id: `tip-${actor.user.login}`,
              kind: 'tip',
              title: logistics ? 'Подготовка к покупке транспорта' : 'Подготовка к сезонным закупкам',
              text: logistics
                ? 'Укажите стоимость транспорта и подготовьте договор поставки. Для оборудования может понадобиться залог.'
                : 'Оцените сезонные расходы и подготовьте выписку с поступлениями по месяцам.',
              created_at: feedCreatedAt,
            },
          ];
          const events: components['schemas']['FeedItem'][] = [...applications.values()]
            .filter((entry) => entry.owner === actor.user.login)
            .flatMap((entry) =>
              (histories.get(entry.app.id) ?? []).map((event) => ({
                id: `${entry.app.id}-${event.id}`,
                kind: 'event' as const,
                title: `Изменение заявки № ${entry.app.number}`,
                text: 'Статус заявки обновлён. Откройте карточку, чтобы узнать следующий шаг.',
                application_id: entry.app.id,
                created_at: event.created_at,
              })),
            );
          const limit = Math.min(100, Math.max(1, Number(url.searchParams.get('limit')) || 30));
          return json(
            200,
            [...events, ...tips].sort((a, b) => b.created_at.localeCompare(a.created_at)).slice(0, limit),
          );
        }
        if (actor && pathname === '/api/stats' && req.method === 'GET') {
          if (actor.user.role === 'client') return problem(403, 'Сводка доступна банку');
          const apps = [...applications.values()].map((entry) => entry.app).filter((app) => app.status !== 'draft');
          const counts = new Map<components['schemas']['ApplicationStatus'], number>();
          for (const app of apps) counts.set(app.status, (counts.get(app.status) ?? 0) + 1);
          const by_status = [...counts].map(([status, count]) => ({ status, count, overdue: 0 }));
          const days = Number(new URL(req.url!, 'http://localhost').searchParams.get('days')) || 30;
          return json(200, {
            days,
            submitted: apps.length,
            by_status,
            decided: 0,
            auto_decided: 0,
            approved: 0,
            decision_time: { median_sec: 0, p90_sec: 0 },
            issued_kop: 0,
          });
        }
        if (actor && pathname === '/api/events' && req.method === 'GET') {
          res.writeHead(200, {
            'Content-Type': 'text/event-stream',
            'Cache-Control': 'no-cache',
            Connection: 'keep-alive',
          });
          res.write(': connected\n\n');
          const stream = {
            login: actor.user.login,
            role: actor.user.role,
            session: token!,
            id: new URL(req.url!, 'http://localhost').searchParams.get('application_id'),
            res,
          };
          streams.add(stream);
          res.on('close', () => streams.delete(stream));
          return;
        }
        if (actor && pathname.startsWith('/api/applications')) {
          const url = new URL(req.url!, 'http://localhost');
          const parts = pathname.split('/');
          const id = parts[3];
          const action = parts[4];
          const entry = id ? applications.get(id) : undefined;
          const visible = (item: { owner: string }) => actor.user.role !== 'client' || item.owner === actor.user.login;
          const next: Partial<Record<App['status'], App['status'][]>> = {
            draft: ['submitted', 'cancelled_by_client'],
            need_info: ['checking', 'cancelled_by_client'],
            approved: ['accepted', 'cancelled_by_client'],
            counter_offer: ['accepted', 'cancelled_by_client'],
            signing: ['issued', 'cancelled_by_client'],
          };
          const view = (app: App): App => ({
            ...app,
            allowed_next:
              actor.user.role === 'client'
                ? (next[app.status] ?? [])
                : actor.user.role === 'manager'
                  ? app.status === 'manual_review'
                    ? [
                        'need_info',
                        ...(app.reasons.some((r) => r.code === 'SF_SOURCE_UNAVAILABLE') ? ['checking' as const] : []),
                      ]
                    : app.status === 'checking'
                      ? ['need_info']
                      : []
                  : app.status === 'manual_review'
                    ? [
                        'approved',
                        'counter_offer',
                        'rejected',
                        'need_info',
                        ...(actor.user.role === 'admin' && app.reasons.some((r) => r.code === 'SF_SOURCE_UNAVAILABLE')
                          ? ['checking' as const]
                          : []),
                      ]
                    : app.status === 'checking'
                      ? ['need_info']
                      : [],
          });
          if (id && (!entry || !visible(entry))) return problem(404, 'Заявка не найдена');
          if (entry && action === 'improvements' && req.method === 'GET') {
            if (actor.user.role !== 'client') return problem(403, 'Сценарии доступны клиенту');
            if (entry.owner === 'build' && entry.app.status === 'rejected')
              return json(200, sample(spec.components.schemas.ImprovementPlan));
            return json(200, { available: false, score: 0, grade: '', version: '2026-10-26.1', items: [] });
          }
          if (entry && action === 'history')
            return json(
              200,
              (histories.get(entry.app.id) ?? []).map((event) =>
                actor.user.role === 'client' ? { ...event, comment: undefined } : event,
              ),
            );
          if (!id && req.method === 'GET') {
            const statuses = url.searchParams.getAll('status').flatMap((value) => value.split(','));
            const grades = url.searchParams.getAll('grade').flatMap((value) => value.split(','));
            const sort = url.searchParams.get('sort') ?? '-created_at';
            if (
              ![
                'created_at',
                '-created_at',
                'updated_at',
                '-updated_at',
                'amount_kop',
                '-amount_kop',
                'due_at',
              ].includes(sort)
            )
              return problem(400, 'Неизвестный порядок сортировки');
            const field = sort.replace(/^-/, '') as 'created_at' | 'updated_at' | 'amount_kop' | 'due_at';
            const direction = sort.startsWith('-') ? -1 : 1;
            const items = [...applications.values()]
              .filter(visible)
              .map(({ app }) => view(app))
              .filter(
                (app) =>
                  (!statuses.length || statuses.includes(app.status)) &&
                  (!grades.length || (app.grade && grades.includes(app.grade))) &&
                  (!url.searchParams.has('product') || app.product === url.searchParams.get('product')),
              )
              .sort((a, b) => {
                const left = a[field],
                  right = b[field];
                if (left === undefined) return right === undefined ? a.id.localeCompare(b.id) : 1;
                if (right === undefined) return -1;
                const order = left < right ? -1 : left > right ? 1 : 0;
                return order * direction || a.id.localeCompare(b.id);
              });
            const offset = Math.max(0, Number(url.searchParams.get('offset')) || 0);
            const limit = Math.min(100, Math.max(1, Number(url.searchParams.get('limit')) || 20));
            return json(200, { items: items.slice(offset, offset + limit), total: items.length });
          }
          if (entry && !action && req.method === 'GET') return json(200, view(entry.app));
          if (actor.user.role !== 'client' && !['decision', 'request-info', 'retry-checks'].includes(action))
            return problem(403, 'Действие доступно клиенту');
          try {
            if (entry && action === 'documents' && req.method === 'POST') {
              if (!['draft', 'need_info'].includes(entry.app.status))
                return problem(409, 'Загрузка недоступна на текущем этапе');
              const chunks: Buffer[] = [];
              let size = 0;
              for await (const chunk of req) {
                size += chunk.length;
                if (size > 11 * 1024 * 1024) return problem(413, 'Файл больше 10 МБ');
                chunks.push(Buffer.from(chunk));
              }
              const request = new Request('http://localhost/upload', {
                method: 'POST',
                headers: { 'Content-Type': req.headers['content-type'] ?? '' },
                body: Buffer.concat(chunks),
              });
              const form = await request.formData();
              const file = form.get('file');
              const kind = form.get('kind') as components['schemas']['DocumentKind'];
              if (!(file instanceof File) || !spec.components.schemas.DocumentKind.enum.includes(kind))
                return problem(400, 'Нужны файл и вид документа');
              if (!file.size || file.size > 10 * 1024 * 1024)
                return problem(413, 'Пустой файл или размер больше 10 МБ');
              if (!['application/pdf', 'image/png', 'image/jpeg'].includes(file.type))
                return problem(400, 'Неподдерживаемый формат');
              const bytes = new Uint8Array(await file.arrayBuffer());
              const document: components['schemas']['Document'] = {
                id: randomUUID(),
                application_id: entry.app.id,
                kind,
                status: ['statement', 'financials'].includes(kind) ? 'skipped' : 'uploaded',
                file_name: file.name,
                content_type: file.type as components['schemas']['Document']['content_type'],
                size_bytes: file.size,
                sha256: createHash('sha256').update(bytes).digest('hex'),
                created_at: new Date().toISOString(),
              };
              if (file.name.startsWith('sample-') && kind === 'statement') {
                try {
                  const truth = JSON.parse(
                    readFileSync(
                      new URL(`../testdata/documents/${entry.app.company.inn}_statement_c.truth.json`, import.meta.url),
                      'utf8',
                    ),
                  );
                  document.extracted = {
                    statement: {
                      inn: truth.inn,
                      period_from: truth.period_from,
                      period_to: truth.period_to,
                      opening_balance_kop: Math.round(truth.opening_balance * 100),
                      turnover_in_kop: Math.round(truth.turnover_in * 100),
                      turnover_out_kop: Math.round(truth.turnover_out * 100),
                      closing_balance_kop: Math.round(truth.closing_balance * 100),
                      monthly_in: truth.monthly_in.map((m: { month: string; amount: number }) => ({
                        month: m.month,
                        amount_kop: Math.round(m.amount * 100),
                      })),
                    },
                  };
                  document.status = 'recognized';
                  document.model = 'demo-fixture';
                } catch {
                  document.status = 'failed';
                  document.error = 'Не удалось прочитать пример';
                }
              }
              documents.set(document.id, { owner: entry.owner, document, bytes });
              entry.app.documents.push({ id: document.id, kind, status: document.status, file_name: file.name });
              publish(entry.owner, entry.app, actor.user.role);
              return json(201, document);
            }
            let raw = '';
            for await (const chunk of req) {
              raw += chunk;
              if (raw.length > 16384) return problem(413, 'Слишком большой запрос');
            }
            const body = raw ? JSON.parse(raw) : {};
            if (entry && ['decision', 'request-info', 'retry-checks'].includes(action) && req.method === 'POST') {
              const app = entry.app;
              const manager = ['manager', 'admin'].includes(actor.user.role);
              const underwriter = ['underwriter', 'admin'].includes(actor.user.role);
              if (action === 'decision') {
                if (!underwriter) return problem(403, 'Решение доступно андеррайтеру');
                if (app.status !== 'manual_review') return problem(409, 'Заявка уже обработана');
                if (
                  !['approved', 'counter_offer', 'rejected'].includes(body.outcome) ||
                  typeof body.comment !== 'string' ||
                  body.comment.trim().length < 3 ||
                  body.comment.length > 2000 ||
                  !Array.isArray(body.reason_codes) ||
                  !body.reason_codes.length ||
                  body.reason_codes.some((code: string) => !spec.components.schemas.ReasonCode.enum.includes(code))
                )
                  return problem(422, 'Нужны исход, причина и комментарий');
                const terms = body.terms;
                if (
                  body.outcome !== 'rejected' &&
                  (!terms ||
                    !spec.components.schemas.ProductCode.enum.includes(terms.product) ||
                    !Number.isSafeInteger(terms.amount_kop) ||
                    terms.amount_kop <= 0 ||
                    !Number.isInteger(terms.term_months) ||
                    terms.term_months < 1 ||
                    terms.term_months > 60 ||
                    !Number.isInteger(terms.rate_bp) ||
                    terms.rate_bp < 100 ||
                    terms.rate_bp > 5000)
                )
                  return problem(422, 'Проверьте условия решения');
                if (
                  terms &&
                  app.recommendation &&
                  Object.entries(app.recommendation).some(([key, value]) => terms[key] !== value)
                ) {
                  const history = histories.get(app.id) ?? [];
                  history.push({
                    id: history.length + 1,
                    kind: 'override',
                    actor_role: actor.user.role,
                    comment: body.comment,
                    created_at: new Date().toISOString(),
                    reason_codes: body.reason_codes,
                  });
                  histories.set(app.id, history);
                }
                app.status = body.outcome;
                app.offer =
                  body.outcome === 'rejected'
                    ? undefined
                    : {
                        ...terms,
                        monthly_payment_kop: 15000000,
                        expires_at: new Date(Date.now() + 180000).toISOString(),
                      };
                app.reasons = body.reason_codes.map((code: components['schemas']['ReasonCode']) => ({
                  code,
                  text:
                    code === 'R_APPROVED'
                      ? 'Заявка одобрена сотрудником банка.'
                      : code === 'R_GRADE_E'
                        ? 'Уровень риска превышает допустимый.'
                        : 'Условия определены после ручного рассмотрения.',
                }));
              } else if (action === 'request-info') {
                if (!manager && !underwriter) return problem(403, 'Недостаточно прав');
                if (!view(app).allowed_next.includes('need_info')) return problem(409, 'Сейчас запрос недоступен');
                if (
                  typeof body.comment !== 'string' ||
                  body.comment.trim().length < 3 ||
                  body.comment.length > 1000 ||
                  (body.documents &&
                    (!Array.isArray(body.documents) ||
                      body.documents.some((kind: string) => !spec.components.schemas.DocumentKind.enum.includes(kind))))
                )
                  return problem(422, 'Проверьте запрос');
                app.status = 'need_info';
                app.info_request = {
                  documents: body.documents ?? [],
                  reasons: [],
                  comment: body.comment,
                  due_at: new Date(Date.now() + 180000).toISOString(),
                };
              } else {
                if (!manager) return problem(403, 'Повтор доступен менеджеру');
                if (app.status !== 'manual_review' || !app.reasons.some((r) => r.code === 'SF_SOURCE_UNAVAILABLE'))
                  return problem(409, 'Повтор проверок недоступен');
                app.status = 'checking';
              }
              app.version++;
              app.updated_at = new Date().toISOString();
              publish(entry.owner, app, actor.user.role, body.comment ?? 'Повтор проверок');
              return json(200, view(app));
            }
            if (entry && req.method === 'GET' && action === 'variant') {
              const offer = entry.app.offer;
              if (entry.app.status !== 'counter_offer' || !offer) return problem(409, 'Только во встречном');
              // упрощение мока: в рамках всё, что не больше встречного и не короче его
              const amount = Number(url.searchParams.get('amount_kop'));
              const term = Number(url.searchParams.get('term_months'));
              const auto = amount <= offer.amount_kop && term >= offer.term_months;
              const [a, n] = auto ? [amount, term] : [offer.amount_kop, offer.term_months];
              const r = offer.rate_bp / 120000;
              const terms = {
                ...offer,
                amount_kop: a,
                term_months: n,
                monthly_payment_kop: Math.round((a * r) / (1 - (1 + r) ** -n)),
              };
              return json(200, { fit: auto ? 'auto' : 'limited', terms });
            }
            if (entry && req.method === 'POST' && ['accept', 'cancel', 'signing-code', 'sign'].includes(action)) {
              const app = entry.app;
              if (action === 'cancel') {
                if (!view(app).allowed_next.includes('cancelled_by_client'))
                  return problem(409, 'Сейчас отменить нельзя');
                app.status = 'cancelled_by_client';
                signing.delete(app.id);
              } else if (action === 'accept') {
                if (!view(app).allowed_next.includes('accepted') || body.version !== app.version)
                  return problem(409, 'Условия или статус изменились');
                if (!app.offer || Date.parse(app.offer.expires_at) <= Date.now())
                  return problem(409, 'Предложение истекло');
                if (body.amount_kop) {
                  if (body.amount_kop > app.offer.amount_kop || body.term_months < app.offer.term_months)
                    return problem(409, 'Такие условия банк автоматически не одобряет');
                  app.offer = { ...app.offer, amount_kop: body.amount_kop, term_months: body.term_months };
                }
                // Подготовка документов в моке мгновенная.
                app.status = 'signing';
              } else {
                if (app.status !== 'signing') return problem(409, 'Подписание сейчас недоступно');
                if (action === 'signing-code') {
                  const code = {
                    code: String(randomInt(100000, 1000000)),
                    expires_at: new Date(Date.now() + 180000).toISOString(),
                    attempts_left: 3,
                  };
                  signing.set(app.id, code);
                  const list = notices.get(entry.owner) ?? [];
                  list.unshift({
                    id: list.length + 1,
                    application_id: app.id,
                    kind: 'signing_code',
                    title: 'Код подписания',
                    text: `Код ${code.code} для подписания договора по заявке №${app.number}`,
                    created_at: new Date().toISOString(),
                  });
                  notices.set(entry.owner, list);
                  return json(200, { expires_at: code.expires_at, attempts_left: code.attempts_left });
                }
                const issued = signing.get(app.id);
                if (!issued || issued.attempts_left === 0 || Date.parse(issued.expires_at) <= Date.now())
                  return problem(422, 'Получите новый код подписания');
                if (body.code !== issued.code) {
                  issued.attempts_left--;
                  return problem(422, 'Неверный код подписания');
                }
                signing.delete(app.id);
                app.status = 'issued';
              }
              app.version++;
              app.updated_at = new Date().toISOString();
              publish(entry.owner, app);
              return json(200, view(app));
            }
            if (entry && action === 'provide-info' && req.method === 'POST') {
              if (entry.app.status !== 'need_info') return problem(409, 'Сейчас уточнения не запрошены');
              const ids = body.document_ids;
              if (
                !Array.isArray(ids) ||
                ids.some(
                  (id) =>
                    !entry.app.documents.some(
                      (doc) =>
                        doc.id === id &&
                        (doc.status === 'confirmed' || !['statement', 'financials'].includes(doc.kind)),
                    ),
                )
              )
                return problem(422, 'Проверьте документы');
              if (
                entry.app.info_request?.documents.some(
                  (kind) => !entry.app.documents.some((doc) => doc.kind === kind && ids.includes(doc.id)),
                )
              )
                return problem(422, 'Приложите все запрошенные документы');
              if (entry.owner === 'it') {
                entry.app.scoring = sample(spec.components.schemas.Scoring) as components['schemas']['Scoring'];
                entry.app.checks = [
                  sample(spec.components.schemas.CheckResult) as components['schemas']['CheckResult'],
                ];
                entry.app.recommendation = {
                  product: entry.app.product,
                  amount_kop: 700000000,
                  term_months: 24,
                  rate_bp: 2250,
                  guarantor_required: true,
                  pledge_required: false,
                  msp_guarantee: false,
                };
                entry.app.policy_version = 'demo';
              }
              entry.app = {
                ...entry.app,
                status: entry.owner === 'it' ? 'manual_review' : 'checking',
                info_request: undefined,
                version: entry.app.version + 1,
                updated_at: new Date().toISOString(),
              };
              publish(entry.owner, entry.app, actor.user.role);
              return json(200, view(entry.app));
            }
            if (entry && !action && req.method === 'PATCH') {
              if (entry.app.status !== 'draft' || body.version !== entry.app.version)
                return problem(409, 'Версия или статус заявки изменились');
              const patch = Object.fromEntries(
                Object.entries(body).filter(
                  ([key]) => key in spec.components.schemas.ApplicationPatch.properties && key !== 'version',
                ),
              );
              const amount = patch.amount_kop ?? entry.app.amount_kop;
              const term = patch.term_months ?? entry.app.term_months;
              if (
                !Number.isSafeInteger(amount) ||
                Number(amount) <= 0 ||
                !Number.isInteger(term) ||
                Number(term) < 1 ||
                Number(term) > 60
              )
                return problem(422, 'Проверьте сумму и срок');
              entry.app = {
                ...entry.app,
                ...patch,
                version: entry.app.version + 1,
                updated_at: new Date().toISOString(),
              };
              publish(entry.owner, entry.app, actor.user.role);
              return json(200, view(entry.app));
            }
            if (!id && req.method === 'POST') {
              const key = req.headers['idempotency-key'];
              if (typeof key !== 'string' || !key) return problem(400, 'Нужен Idempotency-Key');
              const scopedKey = `${actor.user.login}:${key}`;
              const previous = keys.get(scopedKey);
              if (previous)
                return previous.body === raw
                  ? json(201, view(applications.get(previous.id)!.app))
                  : problem(409, 'Ключ уже использован с другими данными');
              const data = body as components['schemas']['NewApplication'];
              if (
                !/^\d{10}(\d{2})?$/.test(data.inn ?? '') ||
                !Number.isSafeInteger(data.amount_kop) ||
                data.amount_kop <= 0 ||
                !Number.isInteger(data.term_months) ||
                data.term_months < 1 ||
                data.term_months > 60 ||
                !data.product ||
                !data.purpose ||
                !data.representative_role
              )
                return problem(422, 'Проверьте поля заявки');
              const now = new Date().toISOString();
              const app: App = {
                ...data,
                id: randomUUID(),
                number: applications.size + 1,
                status: 'draft',
                version: 1,
                company: {
                  ...actor.companies[0].company,
                  inn: data.inn,
                  short_name: companyNames[data.inn] ?? 'Демонстрационная компания',
                },
                bki_consent: data.bki_consent ?? false,
                pd_consent: data.pd_consent ?? false,
                created_at: now,
                updated_at: now,
                allowed_next: [],
                reasons: [],
                documents: [],
              };
              applications.set(app.id, { owner: actor.user.login, app });
              keys.set(scopedKey, { body: raw, id: app.id });
              publish(actor.user.login, app, actor.user.role);
              return json(201, view(app));
            }
            if (entry && action === 'submit' && req.method === 'POST') {
              if (entry.app.status !== 'draft') return problem(409, 'Заявка уже подана');
              if (
                entry.app.representative_role === 'proxy' &&
                !entry.app.documents.some((doc) => doc.kind === 'power_of_attorney')
              )
                return problem(422, 'Приложите доверенность');
              if (!entry.app.bki_consent) return problem(422, 'Нужно согласие на запрос кредитной истории');
              entry.app = {
                ...entry.app,
                status: 'submitted',
                version: entry.app.version + 1,
                submitted_at: new Date().toISOString(),
                updated_at: new Date().toISOString(),
              };
              // Фиксированный сценарий разработки, не реализация кредитной политики.
              if (entry.owner === 'it') {
                entry.app.status = 'need_info';
                entry.app.info_request = {
                  documents: ['statement', 'financials'],
                  reasons: [],
                  comment: 'Приложите и подтвердите выписку и отчётность.',
                  due_at: new Date(Date.now() + 180000).toISOString(),
                };
              } else if (entry.owner === 'coffee' && !entry.app.documents.some((doc) => doc.kind === 'passport')) {
                entry.app.status = 'need_info';
                entry.app.info_request = {
                  documents: ['passport'],
                  reasons: [{ code: 'SF_DOCS_MISSING', text: 'Не хватает документа: паспорт руководителя' }],
                  due_at: new Date(Date.now() + 180000).toISOString(),
                };
              } else if (entry.owner === 'unavailable') {
                entry.app.status = 'manual_review';
                entry.app.reasons = [{ code: 'SF_SOURCE_UNAVAILABLE', text: 'Источник временно недоступен.' }];
                entry.app.checks = [
                  {
                    source: 'egrul',
                    status: 'unavailable',
                    checked_at: new Date().toISOString(),
                    error: 'Реестр не отвечает',
                  },
                ];
              } else if (entry.owner === 'build') {
                entry.app.status = 'rejected';
                entry.app.reasons = [{ code: 'R_GRADE_E', text: 'Риск по заявке превышает допустимый уровень.' }];
              } else if (entry.owner === 'coffee' || entry.owner === 'logistics') {
                entry.app.status = entry.owner === 'coffee' ? 'approved' : 'counter_offer';
                entry.app.offer = {
                  product: entry.app.product,
                  amount_kop: entry.owner === 'coffee' ? entry.app.amount_kop : 390000000,
                  term_months: entry.owner === 'coffee' ? entry.app.term_months : 48,
                  rate_bp: entry.owner === 'coffee' ? 1900 : 2050,
                  monthly_payment_kop: entry.owner === 'coffee' ? 12602000 : 11950000,
                  expires_at: new Date(Date.now() + 180000).toISOString(),
                  guarantor_required: false,
                  msp_guarantee: false,
                  pledge_required: entry.owner === 'logistics',
                  adjustable: entry.owner === 'logistics',
                };
                entry.app.reasons = [
                  {
                    code: entry.owner === 'coffee' ? 'R_APPROVED' : 'R_AMOUNT_DSR',
                    text:
                      entry.owner === 'coffee'
                        ? 'Одобрено на запрошенных условиях.'
                        : 'Предлагаем меньшую сумму, чтобы снизить ежемесячную нагрузку.',
                  },
                ];
              }
              publish(entry.owner, entry.app, actor.user.role);
              return json(200, view(entry.app));
            }
            return problem(501, 'Сценарий пока не реализован в моке');
          } catch {
            return problem(400, 'Некорректный запрос');
          }
        }
        const route = routes.find(({ pattern }) => pattern.test(pathname));
        const operation = route?.operations[req.method?.toLowerCase() ?? 'get'];
        const status = operation && Object.keys(operation.responses).find((code) => /^2\d\d$/.test(code));
        res.setHeader('X-Mock-Api', 'contract-examples');
        if (!status || pathname === '/api/events') {
          res.statusCode = operation ? 501 : 404;
          res.setHeader('Content-Type', 'application/problem+json');
          res.end(JSON.stringify({ type: 'about:blank', title: 'Нет примера ответа', status: res.statusCode }));
          return;
        }
        const content = operation.responses[status].content?.['application/json'];
        res.statusCode = Number(status);
        if (!content) return res.end();
        res.setHeader('Content-Type', 'application/json');
        res.end(JSON.stringify(content.example ?? sample(content.schema)));
      });
    },
  };
}
