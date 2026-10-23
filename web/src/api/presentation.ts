const currency = new Intl.NumberFormat('ru-RU', { style: 'currency', currency: 'RUB' });
export const money = (kopecks: number) => currency.format(kopecks / 100);
export const products = { working_capital: 'Оборотный кредит', overdraft: 'Овердрафт', equipment: 'Кредит на оборудование' };
