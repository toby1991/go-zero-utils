import { describe, expect, test } from 'bun:test';
import { get } from 'svelte/store';
import { createDataTableController } from '../src/lib/controller.js';
import { buildGoZeroQuery, inspectGoZeroPayload, readGoZeroPage } from '../src/lib/go-zero/index.js';
import { createTableUrlCoordinator, parsePage, queryPatch } from '../src/lib/sveltekit/index.js';

function deferred<T>() { let resolve!: (value: T) => void; let reject!: (error: unknown) => void; const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; }
const page = (rows: string[] = []) => ({ rows, page: 1, pageSize: 10, total: rows.length });
function make(read = async (_query: {page:number; code:string}) => page()) {
  return createDataTableController({ initialQuery: { page: 1, code: '' }, defaults: { page: 1, code: '' }, pagination: { pageSize: 10 }, read, errorMessage: () => '安全错误' });
}

describe('go-zero compatibility', () => {
  test('success envelope and legacy raw body', () => {
    for (const code of [0, '0', null, undefined]) expect(inspectGoZeroPayload({code,msg:'OK',data:[]},true)).toEqual({ok:true,data:[]});
    expect(inspectGoZeroPayload([1],true)).toEqual({ok:true,data:[1]});
  });
  test('HTTP errors always win, 200 business errors remain errors', () => {
    for (const payload of ['Unauthorized', {code:-1,msg:'bad'}, {code:0,msg:'OK',data:[]}]) expect(inspectGoZeroPayload(payload,false).ok).toBe(false);
    expect(inspectGoZeroPayload({code:-1,msg:'bad',data:null},true).ok).toBe(false);
  });
  test('strict pagination metadata with valid empty page', () => {
    expect(readGoZeroPage({page:1,pageSize:10,total:0,data:[]})).toEqual({page:1,pageSize:10,total:0,items:[]});
    for (const patch of [{page:0},{page:1.1},{pageSize:-1},{total:-1},{total:Number.MAX_SAFE_INTEGER+1},{data:null},{page:NaN}]) expect(readGoZeroPage({page:1,pageSize:10,total:0,data:[],...patch})).toBeNull();
  });
  test('query preserves false/zero and encodes, omits empty', () => {
    expect(buildGoZeroQuery({a:false,b:0,c:null,d:undefined,e:'',q:'中文 空格'})).toBe('?a=false&b=0&q=%E4%B8%AD%E6%96%87+%E7%A9%BA%E6%A0%BC');
    expect(buildGoZeroQuery({q:''})).toBe('');
  });
});

describe('controller request ownership', () => {
  test('scope null never reads, same query reads once, draft is not query', async () => {
    let calls=0;
    const c=make(async () => { calls++; return page(['a']); });
    await c.sync({page:1,code:''},null); expect(calls).toBe(0);
    await c.sync({page:1,code:''},'session'); await c.sync({page:1,code:''},'session');
    c.patchDraft({code:'vip'}); await c.refresh();
    expect(calls).toBe(2); expect(get(c).draft.code).toBe('vip'); expect(get(c).query.code).toBe('');
    await c.submit(); expect(get(c).query).toEqual({page:1,code:'vip'});
  });
  test('ordinary refresh shares flight; fresh bypasses and old response cannot commit', async () => {
    const a=deferred<ReturnType<typeof page>>(), b=deferred<ReturnType<typeof page>>(); let calls=0;
    const c=make(() => ++calls===1?a.promise:b.promise);
    const first=c.sync({page:1,code:''},'s'); expect(c.refresh()).toBe(first); await Promise.resolve();
    const second=c.refresh({fresh:true}); await Promise.resolve(); expect(calls).toBe(2);
    b.resolve(page(['new'])); expect((await second).status).toBe('success');
    a.resolve(page(['old'])); expect((await first).status).toBe('superseded'); expect(get(c).rows).toEqual(['new']);
  });
  test('errors are safe results; scope change clears rows and invalidates old reads', async () => {
    const wait=deferred<ReturnType<typeof page>>(); const c=make(() => wait.promise);
    const first=c.sync({page:1,code:''},'s'); await c.sync({page:1,code:''},null);
    wait.resolve(page(['private'])); expect((await first).status).toBe('superseded'); expect(get(c).rows).toEqual([]);
    const failing=make(async()=>{throw Error('secret');}); expect((await failing.sync({page:1,code:''},'s')).status).toBe('error'); expect(get(failing).error).toBe('安全错误');
  });
  test('destroy prevents pending result', async () => {
    const wait=deferred<ReturnType<typeof page>>(); const c=make(()=>wait.promise); const first=c.sync({page:1,code:''},'s'); c.destroy(); wait.resolve(page(['a'])); expect((await first).status).toBe('superseded'); expect(get(c).rows).toEqual([]);
  });
  test('destroyed or anonymous controller never initiates navigation', async () => {
    let calls=0; const c=createDataTableController({initialQuery:{page:1},defaults:{page:1},pagination:{pageSize:10},read:async()=>page(),errorMessage:()=>'',navigate:async()=>{calls++;}});
    await c.goToPage(2); await c.sync({page:1},'s'); c.destroy(); await c.goToPage(2); expect(calls).toBe(0);
  });
  test('scope invalidation before read microtask prevents old query using a new session', async () => {
    let calls=0; const c=make(async()=>{calls++;return page();});
    const request=c.sync({page:1,code:''},'s'); await c.sync({page:1,code:''},null); await request; expect(calls).toBe(0);
  });
  test('history restore synchronizes even same query; navigation failure does not commit', async () => {
    const c=createDataTableController({initialQuery:{page:1,code:''},defaults:{page:1,code:''},pagination:{pageSize:10},read:async()=>page(),errorMessage:()=>'',navigate:async()=>{throw Error('navigation');}});
    await c.sync({page:1,code:''},'s'); c.patchDraft({code:'draft'}); await c.sync({page:1,code:''},'s',{restoreDraft:true}); expect(get(c).draft.code).toBe('');
    c.patchDraft({code:'new'}); await expect(c.submit()).rejects.toThrow('navigation'); expect(get(c).query.code).toBe('');
  });
  test('typing during submitted navigation survives route sync', async () => {
    const navigation=deferred<void>();
    const c=createDataTableController({initialQuery:{page:1,code:''},defaults:{page:1,code:''},pagination:{pageSize:10},read:async()=>page(),errorMessage:()=>'',navigate:()=>navigation.promise});
    await c.sync({page:1,code:''},'s'); c.patchDraft({code:'submitted'}); const submitted=c.submit(); c.patchDraft({code:'new draft'});
    await c.sync({page:1,code:'submitted'},'s'); navigation.resolve(); await submitted; expect(get(c).draft.code).toBe('new draft');
  });
  test('non-paginated reader needs no fabricated pagination', async () => {
    const c=createDataTableController<string,{}>({initialQuery:{},defaults:{},pagination:false,read:async()=>({rows:['a']}),errorMessage:()=>''});
    await c.sync({},'s'); expect(get(c).query).toEqual({}); expect(get(c).pagination).toBe(false); expect(get(c).rows).toEqual(['a']);
  });
  test('clear on already-default URL resets an unsubmitted draft without another GET', async () => {
    let calls=0;
    const c=createDataTableController({initialQuery:{page:1,code:''},defaults:{page:1,code:''},pagination:{pageSize:10},read:async()=>{calls++;return page();},errorMessage:()=>'',navigate:async()=>{}});
    await c.sync({page:1,code:''},'s'); c.patchDraft({code:'draft'}); await c.clear(); expect(get(c).draft.code).toBe(''); expect(calls).toBe(1);
  });
  test('page navigation preserves dirty filters but history replaces them', async () => {
    const c=make(async (query)=>({...page(),page:query.page})); await c.sync({page:1,code:'applied'},'s');
    c.patchDraft({code:'draft'}); await c.goToPage(2); expect(get(c).draft.code).toBe('draft'); expect(get(c).query).toEqual({page:2,code:'applied'});
    await c.sync({page:1,code:'applied'},'s',{restoreDraft:true}); expect(get(c).draft.code).toBe('applied');
  });
  test('same-key normalized submit updates draft without an extra read', async () => {
    let calls=0; const c=createDataTableController({initialQuery:{page:1,code:'vip'},defaults:{page:1,code:''},normalize:(q)=>({...q,code:q.code.trim()}),pagination:{pageSize:10},read:async()=>{calls++;return page();},errorMessage:()=>'',navigate:async()=>{}});
    await c.sync({page:1,code:'vip'},'s'); c.patchDraft({code:' vip '}); await c.submit(); expect(get(c).draft.code).toBe('vip'); expect(calls).toBe(1);
  });
  test('pending submit cannot touch another session or a history-restored draft', async () => {
    for (const nextScope of ['other','s']) {
      const wait=deferred<void>(); const c=createDataTableController({initialQuery:{page:1,code:'vip'},defaults:{page:1,code:''},normalize:(q)=>({...q,code:q.code.trim()}),pagination:{pageSize:10},read:async()=>page(),errorMessage:()=>'',navigate:()=>wait.promise});
      await c.sync({page:1,code:'vip'},'s'); c.patchDraft({code:' vip '}); const pending=c.submit();
      await c.sync({page:1,code:'vip'},nextScope,{restoreDraft:true}); c.patchDraft({code:'new session draft'}); wait.resolve(); await pending; expect(get(c).draft.code).toBe('new session draft');
    }
  });
  test('error mapper failure still resolves a safe error result', async () => {
    const c=createDataTableController({initialQuery:{page:1},defaults:{page:1},pagination:{pageSize:10},read:async()=>{throw Error('private');},errorMessage:()=>{throw Error('mapper');}});
    expect((await c.sync({page:1},'s')).status).toBe('error'); expect(get(c).error).toBe('列表读取失败，请重试');
  });
});

describe('URL coordination', () => {
  test('owned keys omit defaults, clear aliases, preserve non-default false and zero', () => {
    expect(queryPatch({page:1,code:'vip',verified:false,id:0},{page:1,code:'',verified:true,id:7},['page','code','verified','id','oldVerified'])).toEqual({page:null,code:'vip',verified:false,id:0,oldVerified:null});
  });
  test('two panels preserve each other, unrelated parameters and hash', async () => {
    let url=new URL('https://example.test/dashboard/?other=x#keep'); const seen:string[]=[];
    const c=createTableUrlCoordinator({getUrl:()=>url,navigate:async(next)=>{seen.push(next.href);url=next;}});
    await Promise.all([c.patch({keysPage:2}),c.patch({historyPage:3})]);
    expect(url.searchParams.get('keysPage')).toBe('2'); expect(url.searchParams.get('historyPage')).toBe('3'); expect(url.searchParams.get('other')).toBe('x'); expect(url.hash).toBe('#keep'); expect(seen).toHaveLength(2);
    await c.patch({keysPage:null}); expect(url.searchParams.has('keysPage')).toBe(false);
  });
  test('cancel discards queued patches and failed navigation does not poison queue', async () => {
    let url=new URL('https://example.test/'); let calls=0;
    const c=createTableUrlCoordinator({getUrl:()=>url,navigate:async(next)=>{calls++; if(calls===1) throw Error('fail');url=next;}});
    await expect(c.patch({page:2})).rejects.toThrow('fail'); await c.patch({page:3}); expect(url.searchParams.get('page')).toBe('3');
    const canceled=c.patch({page:4}); c.cancel(); await canceled; expect(calls).toBe(2);
  });
  test('strict page parsing',()=>{ for(const bad of [null,'','0','-1','1.1','1e2',' 2','99999999999999999999']) expect(parsePage(bad)).toBe(1); expect(parsePage('2')).toBe(2); });
});
