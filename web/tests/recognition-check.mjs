import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL || 'msedge'});
try {
 const ctx=await b.newContext(); const p=await ctx.newPage(); await p.goto('http://127.0.0.1:5175'); await p.getByLabel('Логин',{exact:true}).fill('coffee'); await p.getByLabel('Пароль',{exact:true}).fill('demo12345'); await p.getByRole('button',{name:'Войти',exact:true}).click(); await p.getByRole('heading',{name:'Обзор'}).waitFor();
 const r=await ctx.request.post('http://127.0.0.1:5175/api/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000100012',representative_role:'director',product:'working_capital',purpose:'purchase',amount_kop:250000000,term_months:24}}); const app=await r.json();
 await p.goto('http://127.0.0.1:5175/client/applications/'+app.id); await p.getByRole('heading',{name:'Документы для заявки',exact:true}).waitFor();
 await p.getByRole('button',{name:'Взять пример выписки',exact:true}).click();await p.getByRole('button',{name:'Загрузить',exact:true}).click();
 await p.getByRole('button',{name:'Заполнить из документа',exact:true}).click();
 const closing=p.getByLabel('Остаток на конец, ₽',{exact:true});const before=Number(await closing.inputValue());await closing.fill(String(before+0.01));
 await p.getByRole('button',{name:'Подтвердить выписку',exact:true}).click();await p.getByRole('button',{name:'Подтвердить выписку',exact:true}).waitFor({state:'detached'});
 const appResponse=await ctx.request.get('http://127.0.0.1:5175/api/applications/'+app.id);const card=await appResponse.json();assert.equal(card.documents[0].status,'confirmed');
 const docResponse=await ctx.request.get('http://127.0.0.1:5175/api/documents/'+card.documents[0].id);const doc=await docResponse.json();assert.ok(doc.edited_fields.includes('closing_balance_kop'));assert.equal(doc.model,'demo-fixture');
 const bank=await b.newContext();const bp=await bank.newPage();await bp.goto('http://127.0.0.1:5175');await bp.getByLabel('Логин',{exact:true}).fill('manager');await bp.getByLabel('Пароль',{exact:true}).fill('demo12345');await bp.getByRole('button',{name:'Войти',exact:true}).click();await bp.getByRole('heading',{name:'Очередь банка'}).waitFor();await bp.goto('http://127.0.0.1:5175/bank/applications/'+app.id);await bp.getByRole('row').filter({hasText:'closing_balance_kop (изменено)'}).waitFor();
 const failed=await ctx.request.post('http://127.0.0.1:5175/api/applications/'+app.id+'/documents',{multipart:{kind:'statement',file:{name:'manual.pdf',mimeType:'application/pdf',buffer:Buffer.from('%PDF-1.4 failure fixture')}}});const failedDoc=await failed.json();
 await p.route('**/api/documents/'+failedDoc.id,route=>route.fulfill({status:200,contentType:'application/json',body:JSON.stringify({...failedDoc,status:'failed',error:'Ошибка распознавания'})}));await p.reload();await p.getByRole('alert').filter({hasText:'Ошибка распознавания'}).waitFor();await p.getByRole('button',{name:'Подтвердить выписку',exact:true}).waitFor();
 console.log('Bank comparison and failed recognition manual fallback passed');
 console.log('Recognition fixture: sample upload, explicit prefill, edit, confirmation and edited fields passed');
} finally {await b.close();}


