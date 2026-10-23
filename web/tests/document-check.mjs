import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL || 'msedge'});
try {
 const ctx=await b.newContext(); const p=await ctx.newPage(); await p.goto('http://127.0.0.1:5175'); await p.getByLabel('Логин',{exact:true}).fill('coffee'); await p.getByLabel('Пароль',{exact:true}).fill('demo12345'); await p.getByRole('button',{name:'Войти',exact:true}).click(); await p.getByRole('heading',{name:'Обзор'}).waitFor();
 const r=await ctx.request.post('http://127.0.0.1:5175/api/applications',{headers:{'Idempotency-Key':crypto.randomUUID()},data:{inn:'0000100012',representative_role:'director',product:'working_capital',purpose:'purchase',amount_kop:250000000,term_months:24}}); const app=await r.json();
 await p.goto('http://127.0.0.1:5175/client/applications/'+app.id); await p.getByRole('heading',{name:'Документы для заявки',exact:true}).waitFor();
 await p.locator('input[type=file]').setInputFiles({name:'statement.pdf',mimeType:'application/pdf',buffer:Buffer.from('%PDF-1.4\nmock fixture')});
 await p.getByRole('button',{name:'Загрузить',exact:true}).click(); await p.getByRole('link',{name:'statement.pdf',exact:true}).waitFor();
 const url=await p.getByRole('link',{name:'statement.pdf'}).getAttribute('href'); const file=await ctx.request.get('http://127.0.0.1:5175'+url); assert.equal(file.status(),200); assert.equal((await file.body()).toString(),'%PDF-1.4\nmock fixture');
 const foreign=await b.newContext(); await foreign.request.post('http://127.0.0.1:5175/api/login',{data:{login:'it',password:'demo12345'}}); assert.equal((await foreign.request.get('http://127.0.0.1:5175'+url)).status(),404);
 console.log('Browser upload, list refresh, exact download bytes, foreign file 404 passed');
} finally {await b.close();}


