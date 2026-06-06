(function () {
  // Redirect to login if no token stored (skip on login/verify pages)
  const isAuthPage = /login\.html|auth-verify\.html|home\.html/.test(location.pathname) ||
                     location.pathname === '/auth-verify' ||
                     location.pathname === '/';
  if (!isAuthPage && !localStorage.getItem('phrasely_token')) {
    location.href = 'login.html';
    return;
  }

  const isBubble  = /bubble\.html/.test(location.pathname);
  const isAdd     = /add\.html/.test(location.pathname);
  const isPhrases = /phrases\.html/.test(location.pathname);
  const isIndex   = /index\.html/.test(location.pathname) || location.pathname === '/';

  const nav = document.createElement('nav');
  nav.id = 'navbar';
  nav.innerHTML = `
    <div id="navbar-inner">
      <div id="logo"><a href="index.html"><img src="assets/logo.png" alt="Phrasely"></a></div>
      <a href="bubble.html" class="nav-link${isBubble ? ' active' : ''}">Bubble</a>
      <a href="phrases.html" class="nav-link${isPhrases ? ' active' : ''}">Phrases</a>
      <a href="add.html" class="nav-link${isAdd ? ' active' : ''}">Add</a>
      <input id="search-input" type="search" placeholder="Search…" aria-label="Search phrases">
      <button id="signout-btn" class="nav-link" title="Sign out" aria-label="Sign out">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
          <polyline points="16 17 21 12 16 7"/>
          <line x1="21" y1="12" x2="9" y2="12"/>
        </svg>
      </button>
    </div>
  `;

  nav.setAttribute('aria-label', 'Top navigation');
  document.body.prepend(nav);

  // Bottom tab bar (mobile only — hidden via CSS on desktop)
  const tabBar = document.createElement('nav');
  tabBar.setAttribute('aria-label', 'Bottom navigation');
  tabBar.id = 'tab-bar';
  tabBar.innerHTML = `
    <a href="bubble.html" class="tab${isBubble ? ' active' : ''}" aria-label="Bubble">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="12" cy="12" r="10"/>
        <circle cx="9"  cy="10" r="2" fill="currentColor" stroke="none"/>
        <circle cx="15" cy="8"  r="1.5" fill="currentColor" stroke="none"/>
        <circle cx="15" cy="14" r="1" fill="currentColor" stroke="none"/>
        <circle cx="9"  cy="15" r="1.2" fill="currentColor" stroke="none"/>
      </svg>
      <span>Bubble</span>
    </a>
    <a href="phrases.html" class="tab${isPhrases ? ' active' : ''}" aria-label="Phrases">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
        <polyline points="14 2 14 8 20 8"/>
        <line x1="8" y1="13" x2="16" y2="13"/>
        <line x1="8" y1="17" x2="16" y2="17"/>
      </svg>
      <span>Phrases</span>
    </a>
    <a href="add.html" class="tab${isAdd ? ' active' : ''}" aria-label="Add phrase">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="12" cy="12" r="10"/>
        <line x1="12" y1="8" x2="12" y2="16"/>
        <line x1="8"  y1="12" x2="16" y2="12"/>
      </svg>
      <span>Add</span>
    </a>
    <button class="tab" id="tab-signout" aria-label="Sign out">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
        <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
        <polyline points="16 17 21 12 16 7"/>
        <line x1="21" y1="12" x2="9" y2="12"/>
      </svg>
      <span>Sign out</span>
    </button>
  `;
  document.body.appendChild(tabBar);

  const searchInput = document.getElementById('search-input');

  // Live search on index.html; universal redirect via Enter on all other pages
  searchInput.addEventListener('input', e => {
    if (typeof window.onSearch === 'function') window.onSearch(e.target.value);
  });

  searchInput.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !isIndex) {
      const q = searchInput.value.trim();
      if (q) location.href = `index.html?q=${encodeURIComponent(q)}`;
    }
  });

  const signout = () => { localStorage.removeItem('phrasely_token'); location.href = 'login.html'; };
  document.getElementById('signout-btn').addEventListener('click', signout);
  document.getElementById('tab-signout').addEventListener('click', signout);
})();
