'use strict';
const $ = id => document.getElementById(id);
let deviceKey = '', recentKey = '', expiresAt = null;
async function api(path, body) {
  const response = await fetch(path, body === undefined ? {} : {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || 'The receiver could not complete this request.');
  return data;
}
function showError(error) { $('error').textContent = error.message; $('error').hidden = false; }
function empty(text) { const li=document.createElement('li'); li.className='empty'; li.textContent=text; return li; }
function row(title, detail) {
  const li=document.createElement('li'), div=document.createElement('div'), strong=document.createElement('strong'), small=document.createElement('small');
  strong.textContent=title; small.textContent=detail; div.append(strong,small); li.append(div); return li;
}
async function refresh() {
  try {
    const data=await api('/api/status');
    $('computer').textContent=data.name; $('folder').textContent=data.outputDir;
    $('connection').textContent='Ready to receive'; $('connection').className='ready';
    const selected=$('address').value;
    if (JSON.stringify([...$('address').options].map(o=>o.value))!==JSON.stringify(data.addresses)) {
      $('address').replaceChildren(...data.addresses.map(address=>new Option(address,address)));
      if(data.addresses.includes(selected)) $('address').value=selected;
    }
    $('pair').disabled=!data.addresses.length;
    if(!data.addresses.length) $('connection').textContent='Connect this PC to a local network';
    if(deviceKey!==JSON.stringify(data.devices)) {
      deviceKey=JSON.stringify(data.devices);
      $('devices').replaceChildren(...data.devices.map(device=>{
        const li=row(device.name,'Paired '+new Date(device.pairedAt).toLocaleDateString());
        const button=document.createElement('button'); button.className='secondary'; button.textContent='Unpair'; button.setAttribute('aria-label','Unpair '+device.name);
        button.onclick=async()=>{if(!confirm('Unpair '+device.name+'? It will need a new QR code to send files.'))return;button.disabled=true;try{await api('/api/revoke',{id:device.id});await refresh();}catch(e){showError(e);button.disabled=false;}};
        li.append(button);return li;
      }));
      if(!data.devices.length) $('devices').append(empty('No iPhone paired yet.'));
    }
    if(recentKey!==JSON.stringify(data.recent)) {
      recentKey=JSON.stringify(data.recent);
      $('recent').replaceChildren(...data.recent.map(file=>row(file.name,`${(file.bytes/1048576).toFixed(1)} MB · ${new Date(file.savedAt).toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'})}`)));
      if(!data.recent.length) $('recent').append(empty('Photos you send will appear here.'));
    }
  } catch(e) { $('connection').textContent='Receiver disconnected'; $('connection').className=''; }
  if(expiresAt && Date.now()>=expiresAt){$('qr').hidden=true;$('pairing-link').value='';$('expiry').textContent='Code expired. Create a new one to pair.';expiresAt=null;}
}
$('pair').onclick=async()=>{
  $('pair').disabled=true;$('error').hidden=true;
  try {
    const data=await api('/api/pairing',{host:$('address').value});
    $('qr').src='data:image/png;base64,'+data.qr;$('qr').hidden=false;
    $('pairing-link').value=data.uri;$('pairing-details').hidden=false;
    expiresAt=new Date(data.expiresAt).getTime();
    $('expiry').textContent='One use · expires at '+new Date(data.expiresAt).toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'});
    $('pair').textContent='Create a new pairing code';
  } catch(e){showError(e);} finally{$('pair').disabled=false;}
};
$('copy').onclick=async()=>{try{await navigator.clipboard.writeText($('pairing-link').value);$('copy').textContent='Copied';}catch(e){$('pairing-link').select();showError(new Error('Select and copy the pairing link manually.'));}};
refresh(); setInterval(refresh,3000);
