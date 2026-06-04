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
      <input id="search-input" type="search" placeholder="Search…">
      <button id="signout-btn" class="nav-link" style="margin-left:0.5rem;display:flex;align-items:center;" title="Sign out">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
          <polyline points="16 17 21 12 16 7"/>
          <line x1="21" y1="12" x2="9" y2="12"/>
        </svg>
      </button>
    </div>
  `;

  document.body.prepend(nav);

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

  document.getElementById('signout-btn').addEventListener('click', () => {
    localStorage.removeItem('phrasely_token');
    location.href = 'login.html';
  });
})();
