import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL || 'msedge'});
try {
const ctx=await b.newContext(); const p=await ctx.newPage();
await p.goto('http://127.0.0.1:5175'); await p.getByLabel('Логин',{exact:true}).fill('coffee'); await p.getByLabel('Пароль',{exact:true}).fill('demo12345'); await p.getByRole('button',{name:'Войти',exact:true}).click(); await p.getByRole('heading',{name:'Обзор'}).waitFor();
await p.getByRole('link',{name:'Мои заявки',exact:true}).click(); await p.getByRole('heading',{name:'Мои заявки',exact:true}).waitFor();
const ids=[];
for(let i=0;i<21;i++) { const r=await ctx.request.post('http://127.0.0.1:5175/api/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000100012',representative_role:'director',product:i===0?'equipment':'working_capital',purpose:'purchase',amount_kop:100000+i,term_months:24,bki_consent:true}}); assert.equal(r.status(),201); ids.push((await r.json()).id); }
await p.getByText('Всего заявок: 21. Страница 1',{exact:true}).waitFor(); assert.equal(await p.locator('section > ul').first().getByRole('listitem').count(),20);
await p.getByRole('button',{name:'Далее',exact:true}).click(); await p.getByText('Всего заявок: 21. Страница 2',{exact:true}).waitFor(); assert.equal(await p.locator('section > ul').first().getByRole('listitem').count(),1);
await p.goto('http://127.0.0.1:5175/client/applications/'+ids[0]); await p.getByText('Изменить данные заявки',{exact:true}).click();await p.getByRole('heading',{name:'Изменить черновик'}).waitFor();
const second=await ctx.newPage(); await second.goto(p.url()); await second.getByText('Изменить данные заявки',{exact:true}).click();await second.getByRole('heading',{name:'Изменить черновик'}).waitFor();
await second.getByLabel('Сумма, ₽',{exact:true}).fill('999');
await p.getByLabel('Сумма, ₽',{exact:true}).fill('1500'); await p.getByRole('button',{name:'Сохранить изменения',exact:true}).click(); await p.getByText('Изменения сохранены.',{exact:true}).waitFor(); assert.equal(await p.getByRole('button',{name:'Заменить мой ввод актуальными данными',exact:true}).count(),0);
await second.getByRole('alert').filter({hasText:'Заявка изменилась'}).waitFor(); assert.equal(await second.getByLabel('Сумма, ₽',{exact:true}).inputValue(),'999'); assert.equal(await second.getByRole('button',{name:'Сохранить изменения',exact:true}).isDisabled(),true);
await second.getByRole('button',{name:'Заменить мой ввод актуальными данными',exact:true}).click(); assert.equal(await second.getByLabel('Сумма, ₽',{exact:true}).inputValue(),'1500');
console.log('Browser: pagination, draft edit, two-tab conflict and explicit refresh passed');
} finally {await b.close();}




