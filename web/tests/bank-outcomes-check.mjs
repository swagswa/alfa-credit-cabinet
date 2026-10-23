import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL||'msedge'});
try {
const base='http://127.0.0.1:5175/api';const client=await b.newContext();await client.request.post(base+'/login',{data:{login:'unavailable',password:'demo12345'}});
const staff=await b.newContext();const p=await staff.newPage();await p.goto('http://127.0.0.1:5175');await p.getByLabel('Логин',{exact:true}).fill('underwriter');await p.getByLabel('Пароль',{exact:true}).fill('demo12345');await p.getByRole('button',{name:'Войти',exact:true}).click();await p.getByRole('heading',{name:'Очередь банка'}).waitFor();
for(const [label,status] of [['Одобрить','approved'],['Отказать','rejected']]){
 const r=await client.request.post(base+'/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000500010',representative_role:'director',product:'working_capital',purpose:'purchase',amount_kop:250000000,term_months:24,bki_consent:true}});const app=await r.json();await client.request.post(base+'/applications/'+app.id+'/submit');
 const invalid=await staff.request.post(base+'/applications/'+app.id+'/decision',{data:{outcome:status,comment:'',reason_codes:['R_GRADE_E']}});assert.equal(invalid.status(),422);
 await p.goto('http://127.0.0.1:5175/bank/applications/'+app.id);await p.getByRole('heading',{name:'Решение по заявке'}).waitFor();
 await p.getByText('Предложить другие условия',{exact:true}).click();await p.getByRole('option',{name:label,exact:true}).click();await p.getByLabel('Комментарий к решению',{exact:true}).fill('Результат ручной проверки');await p.getByRole('button',{name:'Сохранить решение',exact:true}).click();await p.getByRole('heading',{name:'Решение по заявке'}).waitFor({state:'detached'});
 const current=await client.request.get(base+'/applications/'+app.id);assert.equal((await current.json()).status,status);
}
console.log('Bank: approval, rejection, mandatory comment passed');
}finally{await b.close()}

