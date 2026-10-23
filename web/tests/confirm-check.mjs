import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL || 'msedge'});
try {
const c=await b.newContext();const p=await c.newPage();const warnings=[];p.on('console',m=>{if(m.type()==='error')warnings.push(m.text())});
await p.goto('http://127.0.0.1:5175'); await p.getByLabel('Логин',{exact:true}).fill('it'); await p.getByLabel('Пароль',{exact:true}).fill('demo12345'); await p.getByRole('button',{name:'Войти',exact:true}).click();await p.getByRole('heading',{name:'Обзор'}).waitFor();
const base='http://127.0.0.1:5175/api';let r=await c.request.post(base+'/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000300011',representative_role:'director',product:'working_capital',purpose:'purchase',amount_kop:800000000,term_months:12,bki_consent:true}});const app=await r.json();await c.request.post(base+'/applications/'+app.id+'/submit');
for(const kind of ['financials','statement']) {r=await c.request.post(base+'/applications/'+app.id+'/documents',{multipart:{kind,file:{name:kind+'.pdf',mimeType:'application/pdf',buffer:Buffer.from('%PDF-1.4 test')}}});assert.equal(r.status(),201);}
await p.goto('http://127.0.0.1:5175/client/applications/'+app.id);
await p.getByLabel('Отчётный год',{exact:true}).fill('2025');
for(const [label,value] of [['Выручка, ₽ (строка 2110)','1000'],['Чистая прибыль, ₽ (строка 2400)','100'],['Долгосрочные обязательства, ₽ (строка 1400)','0'],['Краткосрочные обязательства, ₽ (строка 1500)','100'],['Итог баланса, ₽ (строка 1600)','1000']])await p.getByLabel(label,{exact:true}).fill(value);
await p.getByRole('button',{name:'Подтвердить отчётность',exact:true}).click(); await p.getByRole('button',{name:'Подтвердить отчётность',exact:true}).waitFor({state:'detached'});
await p.getByLabel('Начало периода',{exact:true}).fill('01.01.2026'); await p.getByLabel('Конец периода',{exact:true}).fill('31.01.2026');
for(const [label,value] of [['Остаток на начало, ₽','100'],['Поступления за период, ₽','1000'],['Списания за период, ₽','500'],['Остаток на конец, ₽','600'],['Поступления за 2026-01, ₽','1000']])await p.getByLabel(label,{exact:true}).fill(value);
await p.getByRole('button',{name:'Подтвердить выписку',exact:true}).click();await p.getByRole('button',{name:'Подтвердить выписку',exact:true}).waitFor({state:'detached'});
await p.getByRole('button',{name:'Отправить сведения банку',exact:true}).click();await p.getByText('Рассматривает сотрудник',{exact:true}).waitFor();
console.log('Browser: both manual forms and need_info -> checking passed');console.log('Console errors:',JSON.stringify(warnings));
}finally{await b.close()}



