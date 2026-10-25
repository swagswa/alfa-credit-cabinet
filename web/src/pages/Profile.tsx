import { useEffect, useState, type ReactNode } from 'react';
import { Link } from 'react-router';
import { Plate } from '@alfalab/core-components/plate';
import { PureCell } from '@alfalab/core-components/pure-cell';
import { Typography } from '@alfalab/core-components/typography';
import { api, settle } from '../api/client';
import { labels, money, rate, representatives } from '../api/presentation';
import type { components } from '../api/schema.gen';
import { PageHeading } from '../ui/Workspace';

type Profile = components['schemas']['Profile'];
type Company = components['schemas']['ProfileCompany'];

const bankProducts = { account: 'Расчётный счёт', acquiring: 'Эквайринг', salary: 'Зарплатный проект' };
const day = (value: string) => new Date(value).toLocaleDateString('ru-RU', { dateStyle: 'long' });
// +79000000001 -> +7 900 000-00-01
const phone = (p: string) => p.replace(/^\+7(\d{3})(\d{3})(\d{2})(\d{2})$/, '+7 $1 $2-$3-$4');

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <PureCell tag="div" verticalPadding="compact">
      <PureCell.Content>
        <PureCell.Main>
          <PureCell.Text titleColor="secondary" view="primary-small">
            {label}
          </PureCell.Text>
          <PureCell.Text titleColor="primary" view="component-primary">
            {children}
          </PureCell.Text>
        </PureCell.Main>
      </PureCell.Content>
    </PureCell>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="profile-section">
      <Typography.Title tag="h2" view="xsmall">
        {title}
      </Typography.Title>
      {children}
    </section>
  );
}

function CompanyBlocks({ company: c }: { company: Company }) {
  return (
    <>
      <Section title={c.company.short_name}>
        <div className="profile-fields">
          <Field label="Полное название">{c.full_name}</Field>
          <Field label="ИНН">{c.company.inn}</Field>
          <Field label={c.company.kind === 'IP' ? 'ОГРНИП' : 'ОГРН'}>{c.ogrn}</Field>
          {c.kpp && <Field label="КПП">{c.kpp}</Field>}
          <Field label="Основной вид деятельности">
            {c.okved_main.code} {c.okved_main.name}
          </Field>
          <Field label="Адрес">{c.address}</Field>
          <Field label="Дата регистрации">{day(c.registered_at)}</Field>
          <Field label="Вы действуете как">{representatives[c.representative_role]}</Field>
        </div>
      </Section>
      <Section title="Альфа-Банк">
        {c.bank ? (
          <div className="profile-fields">
            <Field label="Счёт для выдачи кредита">{c.bank.account_mask}</Field>
            <Field label="Клиент банка с">{day(c.bank.client_since)}</Field>
            <Field label="Поступления за 12 месяцев">{money(c.bank.turnover_12m_kop, true)}</Field>
            <Field label="Продукты">{c.bank.products.map((p) => bankProducts[p]).join(', ')}</Field>
          </div>
        ) : c.bank_unavailable ? (
          <Plate view="attention" title="Данные банка сейчас не загрузились">
            Счёт и обороты подтянутся позже, обновите страницу через минуту. Остальные данные профиля актуальны.
          </Plate>
        ) : (
          <Typography.Text tag="p">Расчётного счёта в Альфа-Банке у компании нет.</Typography.Text>
        )}
      </Section>
      <Section title="Кредиты в Альфа-Банке">
        {c.loans.length === 0 ? (
          <Typography.Text tag="p">Действующих кредитов нет.</Typography.Text>
        ) : (
          c.loans.map((l) => (
            <div className="profile-fields" key={l.application_id}>
              <Field label={`Заявка №${l.number}, ${labels[l.status].toLowerCase()}`}>
                <Link to={`/client/applications/${l.application_id}`}>
                  {money(l.amount_kop, true)} на {l.term_months} мес.
                </Link>
              </Field>
              <Field label="Ставка и платёж">
                {rate(l.rate_bp)}, {money(l.monthly_payment_kop, true)} в месяц
              </Field>
              <Field label="Выдан">{day(l.issued_at)}</Field>
              {l.contract_id && (
                <Field label="Договор">
                  <a
                    href={`/api/documents/${encodeURIComponent(l.contract_id)}/file`}
                    target="_blank"
                    rel="noreferrer"
                  >
                    Открыть PDF
                  </a>
                </Field>
              )}
            </div>
          ))
        )}
      </Section>
    </>
  );
}

export function Profile() {
  const [profile, setProfile] = useState<Profile>();
  const [error, setError] = useState('');
  useEffect(
    () =>
      settle(
        api.GET('/profile'),
        ({ data }) => (data ? setProfile(data) : setError('Не удалось загрузить профиль. Обновите страницу.')),
        () => setError('Нет связи с сервером. Обновите страницу.'),
      ),
    [],
  );
  const u = profile?.user;
  return (
    <div className="dashboard">
      <PageHeading
        title="Профиль"
        description="Эти данные банк использует при проверке заявки. Если что-то устарело, обновите в отделении банка."
      />
      {error && <p role="alert">{error}</p>}
      {!u && !error && <p role="status">Загружаем профиль...</p>}
      {u && (
        <>
          <Section title="Вы">
            <div className="profile-fields">
              <Field label="ФИО">{u.full_name}</Field>
              <Field label="Телефон">{u.phone ? phone(u.phone) : 'не указан'}</Field>
              <Field label="Почта">{u.email || 'не указана'}</Field>
            </div>
          </Section>
          {profile.companies.map((c) => (
            <CompanyBlocks key={c.company.id} company={c} />
          ))}
        </>
      )}
    </div>
  );
}
