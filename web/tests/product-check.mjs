import {chromium} from 'playwright';
import assert from 'node:assert/strict';
const b=await chromium.launch({channel:process.env.BROWSER_CHANNEL || 'msedge'});
try {
 const p=await b.newPage(); await p.goto('http://127.0.0.1:5175');
 await p.getByLabel('Логин',{exact:true}).fill('coffee'); await p.getByLabel('Пароль',{exact:true}).fill('demo12345'); await p.getByRole('button',{name:'Войти',exact:true}).click();
 await p.goto('http://127.0.0.1:5175/client/new');
 await p.getByLabel('ИНН компании',{exact:true}).fill('0000100012'); await p.getByRole('button',{name:'Найти компанию'}).click(); await p.getByText('Данные реестра доступны только для чтения.',{exact:true}).waitFor();
 await p.getByText('Согласен на обработку персональных данных для рассмотрения заявки',{exact:true}).click();
 await p.getByLabel('Сумма, ₽',{exact:true}).fill('1'); await p.getByRole('button',{name:'Сохранить черновик',exact:true}).click(); await p.getByRole('alert').filter({hasText:'Сумма или срок'}).waitFor();
 await p.getByLabel('Сумма, ₽',{exact:true}).fill('2500000'); await p.getByRole('button',{name:'Подобрать продукт'}).click(); await p.getByText('Предварительный расчёт, окончательные условия определит банк.',{exact:true}).waitFor();
 await p.getByRole('button',{name:'Сохранить черновик',exact:true}).click(); await p.getByRole('heading',{name:'Заявка № 1',exact:true}).waitFor();
 await p.goto('http://127.0.0.1:5175/client/new');
 await p.getByLabel('ИНН компании',{exact:true}).fill('0000500010'); assert.equal(await p.getByRole('button',{name:'Сохранить черновик',exact:true}).isDisabled(),true);
 await p.getByRole('button',{name:'Найти компанию'}).click(); await p.getByRole('alert').filter({hasText:'Реестр временно недоступен'}).waitFor();
 console.log('Browser: company lookup, product limits, recommendation, create, INN change and registry failure passed');
} finally {await b.close();}



