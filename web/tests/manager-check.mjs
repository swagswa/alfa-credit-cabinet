import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL||'msedge'});
try {
const base='http://127.0.0.1:5175/api';const client=await b.newContext();await client.request.post(base+'/login',{data:{login:'unavailable',password:'demo12345'}});let r=await client.request.post(base+'/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000500010',representative_role:'director',product:'working_capital',purpose:'purchase',amount_kop:250000000,term_months:24,bki_consent:true}});const app=await r.json();await client.request.post(base+'/applications/'+app.id+'/submit');
const ctx=await b.newContext();const p=await ctx.newPage();await p.goto('http://127.0.0.1:5175');await p.getByLabel('Логин',{exact:true}).fill('manager');await p.getByLabel('Пароль',{exact:true}).fill('demo12345');await p.getByRole('button',{name:'Войти',exact:true}).click();await p.getByRole('heading',{name:'Очередь банка'}).waitFor();await p.goto('http://127.0.0.1:5175/bank/applications/'+app.id);
assert.equal(await p.getByRole('heading',{name:'Решение по заявке'}).count(),0);
await p.getByRole('button',{name:'Повторить проверки',exact:true}).click();await p.getByText('Проверяем сведения',{exact:true}).waitFor();
await p.getByLabel('Комментарий к запросу',{exact:true}).fill('Приложите обновлённые документы');await p.getByRole('button',{name:'Отправить запрос клиенту',exact:true}).click();await p.getByText('Нужно дополнить сведения',{exact:true}).waitFor();
r=await client.request.get(base+'/applications/'+app.id);assert.equal((await r.json()).info_request.comment,'Приложите обновлённые документы');
const underwriter=await b.newContext();await underwriter.request.post(base+'/login',{data:{login:'underwriter',password:'demo12345'}});
r=await underwriter.request.post(base+'/applications/'+app.id+'/retry-checks');assert.equal(r.status(),403);
await p.setViewportSize({width:390,height:844});assert.equal(await p.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
console.log('Manager: retry, request info, permissions, mobile layout passed');
}finally{await b.close()}

