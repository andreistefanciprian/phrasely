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
  function element() { return {innerHTML:'',textContent:'',value:'',style:{},dataset:{},attributes:{},children:[],events:{},currentTime:0,classList:{toggle(){},add(){},remove(){}},addEventListener(event, fn){this.events[event]=fn},appendChild(el){this.children.push(el)},setAttribute(name,value){this.attributes[name]=value},removeAttribute(name){delete this.attributes[name];if(name==='src')this.src=''},querySelector(){return this.label||(this.label=element())},closest(){return null},pause(){this.paused=true},play(){this.paused=false;return Promise.resolve()},getContext(){return {measureText(text){return {width:text.length*8}}}}}; }
  const listenButtons = [element(), element()];
  const document = {activeElement:null,events:{},getElementById(id){if(!elements.has(id))elements.set(id,element());return elements.get(id)},createElement:element,addEventListener(event,fn){this.events[event]=fn},querySelectorAll(selector){return selector==='.listen-button'?listenButtons:[]},querySelector(){return element()}};
  const context=vm.createContext({document,console,URL,URLSearchParams,setTimeout,clearTimeout,PhraselyHeadwords:{format,key},location:{search:'',origin:'http://localhost'},history:{pushState(){}},localStorage:{getItem(){return null},setItem(){}},window:{innerWidth:1000,innerHeight:800,addEventListener(){}},fetch:async()=>({ok:true,json:async()=>[]})});
  const source=read(`templates/${name}.html`).replace('{{.PhrasesJSON}}',JSON.stringify([sample]));
  for(const match of source.matchAll(/<script>([\s\S]*?)<\/script>/g)) vm.runInContext(match[1],context);
  return {context,elements,document,listenButtons};
}
test('Bubble and Shuffle execute with structured collection data',()=>{
  const bubble=pageContext('bubble');
  assert.equal(bubble.elements.get('bubble').children[0].textContent,'stand up to scrutiny');
  const shuffle=pageContext('shuffle');
  assert.ok(shuffle.elements.get('phrase').innerHTML.includes('(remained convincing)'));
  assert.equal(shuffle.elements.get('keyword').children[0].textContent,'stand up to scrutiny');
});
test('Shuffle Listen controls playback without shuffling and resets on phrase change',async()=>{
  const page=pageContext('shuffle');
  const audio=page.elements.get('phrase-audio');
  const main=page.elements.get('main-content');
  let prevented=false;
  main.events.click({target:{closest(selector){return selector==='.listen-button'?{}:null}},preventDefault(){prevented=true}});
  assert.equal(prevented,false);
  assert.equal(page.listenButtons[0].dataset.state,'rest');

  await page.listenButtons[0].events.click();
  assert.equal(audio.src,'/fd/phrases/sample/audio');
  assert.equal(page.listenButtons[0].dataset.state,'speaking');
  assert.equal(page.listenButtons[0].label.textContent,'STOP');

  vm.runInContext('show(phrases[0])',page.context);
  assert.equal(audio.src,'');
  assert.equal(audio.currentTime,0);
  assert.equal(page.listenButtons[0].dataset.state,'rest');
  assert.equal(page.elements.get('audio-error').textContent,'');
});
test('Shuffle Listen ignores stale playback and reports a generic failure',async()=>{
  const page=pageContext('shuffle');
  const audio=page.elements.get('phrase-audio');
  let rejectPlay;
  audio.play=()=>new Promise((resolve,reject)=>{rejectPlay=reject});
  const pending=page.listenButtons[0].events.click();
  assert.equal(page.listenButtons[0].dataset.state,'loading');
  assert.equal(page.listenButtons[0].attributes['aria-busy'],'true');
  rejectPlay(new Error('failed'));
  await pending;
  assert.equal(page.listenButtons[0].dataset.state,'rest');
  assert.equal(page.elements.get('audio-error').textContent,"Couldn't play this phrase. Try again.");
});
test('Shuffle Listen keyboard focus bypasses the global shuffle shortcut',()=>{
  const page=pageContext('shuffle');
  page.document.activeElement={closest(selector){return selector==='a, button'?{}:null}};
  for(const key of [' ', 'ArrowRight']) {
    let prevented=false;
    page.document.events.keydown({key,preventDefault(){prevented=true}});
    assert.equal(prevented,false);
  }
});
test('Shuffle Listen stale completion cannot stop newer playback',async()=>{
  const page=pageContext('shuffle');
  const audio=page.elements.get('phrase-audio');
  let resolveFirst;
  let playCount=0;
  audio.play=()=>{ audio.paused=false; return ++playCount===1 ? new Promise(resolve=>{resolveFirst=resolve}) : Promise.resolve(); };

  const stale=page.listenButtons[0].events.click();
  vm.runInContext('show(phrases[0])',page.context);
  await page.listenButtons[0].events.click();
  assert.equal(audio.paused,false);

  resolveFirst();
  await stale;
  assert.equal(audio.paused,false);
  assert.equal(page.listenButtons[0].dataset.state,'speaking');
});
test('Shuffle Listen includes the specified light theme and reduced-motion styles',()=>{
  const source=read('templates/shuffle.html');
  assert.match(source,/\.listen-button, \.ask-chatgpt-button \{[^}]*color: var\(--secondary\)/);
  assert.match(source,/\.listen-button:hover, \.listen-button:focus-visible,[\s\S]*?\.ask-chatgpt-button:hover, \.ask-chatgpt-button:focus-visible \{[^}]*background: var\(--surface-subtle\)/);
  assert.match(source,/@media \(prefers-reduced-motion: reduce\) \{ \.listen-button \.speaker-arc \{ animation: none !important; \} \}/);
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
