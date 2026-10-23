import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL||'msedge'});
try {
const pages=[];
for(const login of ['coffee','logistics']) {
 const ctx=await b.newContext(),p=await ctx.newPage();
 await p.goto('http://127.0.0.1:5175');await p.getByLabel('Логин',{exact:true}).fill(login);await p.getByLabel('Пароль',{exact:true}).fill('demo12345');await p.getByRole('button',{name:'Войти',exact:true}).click();
 await p.getByRole('heading',{name:'События',exact:true}).waitFor();await p.getByText('Пока нет событий',{exact:true}).waitFor();
 assert.equal(await p.getByRole('heading',{name:/Подготовка к сезонным|Подготовка к покупке/}).count(),0);
 pages.push({p,ctx});
}
const {p,ctx}=pages[0];const r=await ctx.request.post('http://127.0.0.1:5175/api/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000100012',product:'working_capital',purpose:'purchase',representative_role:'director',amount_kop:250000000,term_months:24}});const app=await r.json();
await p.getByRole('heading',{name:'Изменение заявки № 1',exact:true}).waitFor();await p.getByRole('link',{name:'Открыть заявку',exact:true}).click();assert.ok(p.url().endsWith(app.id));
const other=await pages[1].ctx.request.get('http://127.0.0.1:5175/api/feed');assert.equal((await other.json()).some(item=>item.application_id===app.id),false);
await p.goto('http://127.0.0.1:5175/client');await p.getByRole('heading',{name:'События',exact:true}).waitFor();await p.route('**/api/feed*',route=>route.fulfill({status:503,contentType:'application/problem+json',body:'{}'}));await p.getByRole('button',{name:'Обновить события',exact:true}).click();
await p.getByRole('alert').filter({hasText:'Не удалось загрузить события'}).waitFor();await p.getByRole('heading',{name:'Обзор',exact:true}).waitFor();
console.log('Feed: events only, empty state, live event, card link, ownership and isolated error passed');
}finally{await b.close()}
