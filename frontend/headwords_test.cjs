const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const read = file => fs.readFileSync(path.join(__dirname,file),'utf8');
const {format, key} = require('./static/headwords.js');
const w = (text, meaning, canonical = text) => ({text, meaning, canonical});
test('multiple meanings, repetitions, and real parentheses', () => {
  const html = format('Candid (even then), tactful and candid.', [w('candid', 'honest'), w('tactful', 'careful')]);
  assert.equal(html.match(/honest/g).length, 1);
  assert.equal(html.match(/<strong>/g).length, 3);
  assert.ok(html.includes('</span> (even then)'));
  assert.ok(html.includes('(careful)'));
});
test('literal punctuation, Unicode boundaries, overlaps, unmatched gloss', () => {
  const html = format('Élan and élan; stood up to scrutiny.', [w('élan', 'energy'), w('stood up', 'resisted'), w('stood up to scrutiny', 'remained convincing'), w('missing', 'absent')]);
  assert.equal(html.match(/energy/g).length, 1);
  assert.ok(html.includes('<strong>stood up to scrutiny</strong>'));
  assert.ok(html.includes('(stood up: resisted)'));
  assert.ok(html.includes('(missing: absent)'));
  assert.ok(!format('scathing', [w('cat', 'animal')]).includes('<strong>'));
  assert.ok(format('C++ is useful.', [w('C++', 'a language')]).includes('<strong>C++</strong>'));
});
test('sentence, text and gloss remain escaped', () => {
  const html = format('<script> & word (real)', [w('word', '<img onerror="x">'), w('<b>', '<evil>')]);
  assert.ok(!html.includes('<script>'));
  assert.ok(!html.includes('<img'));
  assert.ok(html.includes('&lt;img'));
  assert.ok(html.includes('(real)'));
});
test('grouping compares canonical sets independently of order and sense', () => {
  assert.equal(key([w('ran', 'moved', 'Run'),w('runs', 'operates', 'run'),w('up', 'higher')]), key([w('up', 'awake'),w('running', 'operating', 'run')]));
});
test('MCP carries the same renderer for offline inline cards', () => {
  const ui = read('../mcp/ui/phrase-choices.html');
  assert.ok(ui.includes(read('static/headwords.js')));
  assert.ok(ui.includes('w.text.trim() !== ""'));
  assert.ok(ui.includes('w.canonical.trim() !== ""'));
  assert.ok(ui.includes('w.meaning.trim() !== ""'));
  assert.ok(ui.includes('validChoiceSourceURL(w.source_url)'));
});

const vm = require('node:vm');
const sample = {id:'sample',phrase:'She stood up to scrutiny (even then).',headwords:[w('stood up to scrutiny','remained convincing','stand up to scrutiny')],note:'Usage note'};
function pageContext(name) {
  const elements = new Map();
  function element() { return {innerHTML:'',textContent:'',value:'',style:{},children:[],events:{},classList:{toggle(){},add(){},remove(){}},addEventListener(event, fn){this.events[event]=fn},appendChild(el){this.children.push(el)},getContext(){return {measureText(text){return {width:text.length*8}}}}}; }
  const document = {getElementById(id){if(!elements.has(id))elements.set(id,element());return elements.get(id)},createElement:element,addEventListener(){},querySelectorAll(){return []},querySelector(){return element()}};
  const context=vm.createContext({document,console,URL,URLSearchParams,setTimeout,clearTimeout,PhraselyHeadwords:{format,key},location:{search:'',origin:'http://localhost'},history:{pushState(){}},localStorage:{getItem(){return null},setItem(){}},window:{innerWidth:1000,innerHeight:800,addEventListener(){}},fetch:async()=>({ok:true,json:async()=>[]})});
  const source=read(`templates/${name}.html`).replace('{{.PhrasesJSON}}',JSON.stringify([sample]));
  for(const match of source.matchAll(/<script>([\s\S]*?)<\/script>/g)) vm.runInContext(match[1],context);
  return {context,elements,document};
}
test('Bubble and Shuffle execute with structured collection data',()=>{
  const bubble=pageContext('bubble');
  assert.equal(bubble.elements.get('bubble').children[0].textContent,'stand up to scrutiny');
  const shuffle=pageContext('shuffle');
  assert.ok(shuffle.elements.get('phrase').innerHTML.includes('(remained convincing)'));
  assert.equal(shuffle.elements.get('keyword').children[0].textContent,'stand up to scrutiny');
});
test('phrase list renders editable per-expression fields and sends replacement objects',async()=>{
  const page=pageContext('phrases');
  const html=page.elements.get('phrase-list').innerHTML;
  for(const field of ['text','canonical','meaning','source_url']) assert.ok(html.includes(`data-field="${field}"`));
  page.document.getElementById('ef-phrase-sample').value=sample.phrase;
  page.document.getElementById('ef-note-sample').value=sample.note;
  page.document.querySelectorAll=()=>[{querySelectorAll:()=>Object.entries(sample.headwords[0]).map(([field,value])=>({dataset:{field},value}))}];
  let payload;
  page.context.fetch=async(url,options)=>{payload=JSON.parse(options.body);return {ok:true,json:async()=>sample}};
  await vm.runInContext('saveEdit("sample")',page.context);
  assert.deepEqual(payload,{phrase:sample.phrase,headwords:sample.headwords,note:sample.note});
});
test('curation preview and Save keep sentence and structured meanings separate',async()=>{
  const page=pageContext('add');
  page.document.getElementById('input').value='stood up to scrutiny';
  page.context.fetch=async()=>({ok:true,json:async()=>sample});
  await page.elements.get('curate-btn').events.click();
  assert.ok(page.elements.get('preview-phrase').innerHTML.includes('(remained convincing)'));
  let payload;
  page.context.fetch=async(url,options)=>{payload=JSON.parse(options.body);return {ok:true,json:async()=>sample}};
  await page.elements.get('save-btn').events.click();
  assert.deepEqual(payload,{phrase:sample.phrase,headwords:sample.headwords,note:sample.note});
});
