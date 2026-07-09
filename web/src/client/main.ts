/**
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * Client entry point
 *
 * Handles client-side routing and real-time state management via SSE.
 */

import themeCSS from '../styles/theme.css?inline';
import '@shoelace-style/shoelace/dist/themes/light.css';
import '@shoelace-style/shoelace/dist/themes/dark.css';

import type { PageData, User } from '../shared/types.js';
import { stateManager } from './state.js';
import { debugLog } from './debug-log.js';
import { setDocumentTitle } from './page-title.js';

// Inject theme CSS so it loads regardless of whether the page is served by
// the Vite dev server (index.html) or the Go SPA shell template (web.go).
{
  const style = document.createElement('style');
  style.textContent = themeCSS;
  document.head.appendChild(style);
}

// Import Shoelace base path config (needed for icons).
// Icons are copied to public/shoelace/ by scripts/copy-shoelace-icons.mjs
// so they are available under both the Vite dev server and the Go server.
import { setBasePath } from '@shoelace-style/shoelace/dist/utilities/base-path.js';
setBasePath('/shoelace');

// Explicitly import all Shoelace components used in the app.
// The autoloader cannot detect sl-* elements inside LitElement shadow roots,
// so each component must be registered via direct import.
import '@shoelace-style/shoelace/dist/components/breadcrumb/breadcrumb.js';
import '@shoelace-style/shoelace/dist/components/breadcrumb-item/breadcrumb-item.js';
import '@shoelace-style/shoelace/dist/components/button/button.js';
import '@shoelace-style/shoelace/dist/components/checkbox/checkbox.js';
import '@shoelace-style/shoelace/dist/components/drawer/drawer.js';
import '@shoelace-style/shoelace/dist/components/icon/icon.js';
import '@shoelace-style/shoelace/dist/components/icon-button/icon-button.js';
import '@shoelace-style/shoelace/dist/components/input/input.js';
import '@shoelace-style/shoelace/dist/components/option/option.js';
import '@shoelace-style/shoelace/dist/components/select/select.js';
import '@shoelace-style/shoelace/dist/components/spinner/spinner.js';
import '@shoelace-style/shoelace/dist/components/progress-bar/progress-bar.js';
import '@shoelace-style/shoelace/dist/components/textarea/textarea.js';
import '@shoelace-style/shoelace/dist/components/tooltip/tooltip.js';
import '@shoelace-style/shoelace/dist/components/dialog/dialog.js';
import '@shoelace-style/shoelace/dist/components/divider/divider.js';
import '@shoelace-style/shoelace/dist/components/dropdown/dropdown.js';
import '@shoelace-style/shoelace/dist/components/menu/menu.js';
import '@shoelace-style/shoelace/dist/components/menu-item/menu-item.js';
import '@shoelace-style/shoelace/dist/components/alert/alert.js';
import '@shoelace-style/shoelace/dist/components/radio-group/radio-group.js';
import '@shoelace-style/shoelace/dist/components/radio-button/radio-button.js';
import '@shoelace-style/shoelace/dist/components/switch/switch.js';
import '@shoelace-style/shoelace/dist/components/tab-group/tab-group.js';
import '@shoelace-style/shoelace/dist/components/tab/tab.js';
import '@shoelace-style/shoelace/dist/components/tab-panel/tab-panel.js';

// Import app shell and core shared components (always needed)
import '../components/app-shell.js';
import '../components/shared/nav.js';
import '../components/shared/header.js';
import '../components/shared/breadcrumb.js';
import '../components/shared/status-badge.js';
import '../components/shared/debug-panel.js';

// Profile shell (lazy-loaded with profile routes)
// import '../components/profile/profile-shell.js';
// import '../components/profile/profile-nav.js';

// Page components are lazy-loaded per route — see ROUTES below.

/** Current authenticated user, fetched once on init */
let currentUser: User | null = null;

/** SSR-prefetched page data, consumed once on initial render */
let ssrPageData: PageData | null = null;

/**
 * Fetch the current authenticated user from the backend session.
 * Returns null if not authenticated.
 */
async function fetchCurrentUser(): Promise<User | null> {
  try {
    const res = await fetch('/auth/me', { credentials: 'include' });
    if (!res.ok) return null;
    const data = await res.json();
    return {
      id: data.id,
      email: data.email,
      name: data.displayName || data.name || '',
      avatar: data.avatarUrl || data.avatar,
      role: data.role || undefined,
    };
  } catch {
    return null;
  }
}

/**
 * Route configuration mapping URL patterns to page component tag names.
 * Each route includes a lazy loader that dynamically imports the page module,
 * which registers its custom element as a side effect.
 */
interface RouteConfig {
  pattern: RegExp;
  tag: string;
  load: () => Promise<unknown>;
}

const ROUTES: RouteConfig[] = [
  { pattern: /^\/login$/, tag: 'fabric-login-page', load: () => import('../components/pages/login.js') },
  { pattern: /^\/invite$/, tag: 'fabric-page-invite', load: () => import('../components/pages/invite.js') },
  { pattern: /^\/onboarding$/, tag: 'fabric-page-onboarding', load: () => import('../components/pages/onboarding.js') },
  { pattern: /^\/$/, tag: 'fabric-page-home', load: () => import('../components/pages/home.js') },
  { pattern: /^\/projects$/, tag: 'fabric-page-projects', load: () => import('../components/pages/projects.js') },
  { pattern: /^\/agents$/, tag: 'fabric-page-agents', load: () => import('../components/pages/agents.js') },
  { pattern: /^\/brokers$/, tag: 'fabric-page-brokers', load: () => import('../components/pages/brokers.js') },
  { pattern: /^\/brokers\/[^/]+$/, tag: 'fabric-page-broker-detail', load: () => import('../components/pages/broker-detail.js') },
  { pattern: /^\/skills$/, tag: 'fabric-page-skills', load: () => import('../components/pages/skills.js') },
  { pattern: /^\/skills\/new$/, tag: 'fabric-page-skill-create', load: () => import('../components/pages/skill-create.js') },
  { pattern: /^\/skills\/[^/]+$/, tag: 'fabric-page-skill-detail', load: () => import('../components/pages/skill-detail.js') },
  { pattern: /^\/admin\/skill-registries$/, tag: 'fabric-page-admin-skill-registries', load: () => import('../components/pages/admin-skill-registries.js') },
  { pattern: /^\/admin\/skill-registries\/[^/]+$/, tag: 'fabric-page-admin-skill-registry-detail', load: () => import('../components/pages/admin-skill-registry-detail.js') },
  { pattern: /^\/admin\/scheduler$/, tag: 'fabric-page-admin-scheduler', load: () => import('../components/pages/admin-scheduler.js') },
  { pattern: /^\/admin\/users$/, tag: 'fabric-page-admin-users', load: () => import('../components/pages/admin-users.js') },
  { pattern: /^\/admin\/groups$/, tag: 'fabric-page-admin-groups', load: () => import('../components/pages/admin-groups.js') },
  { pattern: /^\/admin\/groups\/[^/]+$/, tag: 'fabric-page-admin-group-detail', load: () => import('../components/pages/admin-group-detail.js') },
  { pattern: /^\/metrics$/, tag: 'fabric-page-metrics', load: () => import('../components/pages/metrics-dashboard.js') },
  { pattern: /^\/admin\/metrics$/, tag: 'fabric-page-metrics', load: () => import('../components/pages/metrics-dashboard.js') },
  { pattern: /^\/admin\/maintenance$/, tag: 'fabric-page-admin-maintenance', load: () => import('../components/pages/admin-maintenance.js') },
  { pattern: /^\/admin\/integrations$/, tag: 'fabric-page-admin-integrations', load: () => import('../components/pages/admin-integrations.js') },
  { pattern: /^\/admin\/integrations\/[^/]+$/, tag: 'fabric-page-admin-integrations', load: () => import('../components/pages/admin-integrations.js') },
  { pattern: /^\/admin\/server-config$/, tag: 'fabric-page-admin-server-config', load: () => import('../components/pages/admin-server-config.js') },
  { pattern: /^\/settings$/, tag: 'fabric-page-settings', load: () => import('../components/pages/settings.js') },
  { pattern: /^\/settings\/templates\/[^/]+$/, tag: 'fabric-page-template-detail', load: () => import('../components/pages/template-detail.js') },
  { pattern: /^\/settings\/harness-configs\/[^/]+$/, tag: 'fabric-page-harness-config-detail', load: () => import('../components/pages/harness-config-detail.js') },
  { pattern: /^\/profile\/env$/, tag: 'fabric-page-profile-env-vars', load: () => import('../components/pages/profile-env-vars.js') },
  { pattern: /^\/profile\/secrets$/, tag: 'fabric-page-profile-secrets', load: () => import('../components/pages/profile-secrets.js') },
  { pattern: /^\/profile\/settings$/, tag: 'fabric-page-profile-settings', load: () => import('../components/pages/profile-settings.js') },
  { pattern: /^\/profile\/tokens$/, tag: 'fabric-page-profile-tokens', load: () => import('../components/pages/profile-tokens.js') },
  { pattern: /^\/profile\/telegram$/, tag: 'fabric-page-profile-telegram', load: () => import('../components/pages/profile-telegram.js') },
  { pattern: /^\/profile\/discord$/, tag: 'fabric-page-profile-discord', load: () => import('../components/pages/profile-discord.js') },
  { pattern: /^\/profile$/, tag: 'fabric-page-profile-env-vars', load: () => import('../components/pages/profile-env-vars.js') },
  { pattern: /^\/github-app\/installed$/, tag: 'fabric-page-github-app-setup', load: () => import('../components/pages/github-app-setup.js') },
  { pattern: /^\/projects\/new$/, tag: 'fabric-page-project-create', load: () => import('../components/pages/project-create.js') },
  { pattern: /^\/projects\/[^/]+\/settings$/, tag: 'fabric-page-project-settings', load: () => import('../components/pages/project-settings.js') },
  { pattern: /^\/projects\/[^/]+\/templates\/[^/]+$/, tag: 'fabric-page-template-detail', load: () => import('../components/pages/template-detail.js') },
  { pattern: /^\/projects\/[^/]+\/harness-configs\/[^/]+$/, tag: 'fabric-page-harness-config-detail', load: () => import('../components/pages/harness-config-detail.js') },
  { pattern: /^\/projects\/[^/]+\/schedules$/, tag: 'fabric-page-project-schedules', load: () => import('../components/pages/project-schedules.js') },
  { pattern: /^\/projects\/[^/]+\/metrics$/, tag: 'fabric-page-metrics', load: () => import('../components/pages/metrics-dashboard.js') },
  { pattern: /^\/projects\/[^/]+$/, tag: 'fabric-page-project-detail', load: () => import('../components/pages/project-detail.js') },
  { pattern: /^\/agents\/new$/, tag: 'fabric-page-agent-create', load: () => import('../components/pages/agent-create.js') },
  { pattern: /^\/agents\/[^/]+\/configure$/, tag: 'fabric-page-agent-configure', load: () => import('../components/pages/agent-configure.js') },
  { pattern: /^\/agents\/[^/]+\/terminal$/, tag: 'fabric-page-terminal', load: () => import('../components/pages/terminal.js') },
  { pattern: /^\/agents\/[^/]+$/, tag: 'fabric-page-agent-detail', load: () => import('../components/pages/agent-detail.js') },
];

/**
 * Routes that render without the app shell (full-page layout)
 */
const STANDALONE_ROUTES = new Set(['fabric-login-page', 'fabric-page-invite', 'fabric-page-onboarding']);

/**
 * Routes that render inside the profile shell instead of the main app shell
 */
const PROFILE_ROUTES = new Set(['fabric-page-profile-env-vars', 'fabric-page-profile-secrets', 'fabric-page-profile-settings', 'fabric-page-profile-tokens', 'fabric-page-profile-telegram', 'fabric-page-profile-discord']);

/**
 * Routes that require admin role. Non-admin users are redirected to dashboard.
 */
const ADMIN_ROUTES = new Set(['fabric-page-settings', 'fabric-page-admin-scheduler', 'fabric-page-admin-maintenance', 'fabric-page-admin-users', 'fabric-page-admin-groups', 'fabric-page-admin-group-detail', 'fabric-page-admin-server-config', 'fabric-page-admin-integrations', 'fabric-page-admin-skill-registries', 'fabric-page-admin-skill-registry-detail']);

/**
 * Initialize the client-side application
 */
async function init(): Promise<void> {
  console.info('[Fabric] Initializing client...');

  // Get initial data from SSR and hydrate state manager
  const initialData = getInitialData();
  if (initialData) {
    console.info('[Fabric] Initial page data:', initialData.path);
    if (initialData.user) {
      currentUser = initialData.user;
    }
    // Preserve the full SSR payload so page components can use prefetched data.
    ssrPageData = initialData;
    if (initialData.data) {
      const pageDataObj = initialData.data as {
        agents?: import('../shared/types.js').Agent[];
        projects?: import('../shared/types.js').Project[];
        _capabilities?: import('../shared/types.js').Capabilities;
      };
      stateManager.hydrate(pageDataObj, pageDataObj._capabilities);
    }
  }

  // Attach debug logger to state manager to capture all SSE events
  debugLog.attach(stateManager);

  // Fetch current user from session if not provided by SSR
  if (!currentUser) {
    currentUser = await fetchCurrentUser();
  }

  // Wait for core shell components to be defined (page components are lazy-loaded)
  await Promise.all([
    customElements.whenDefined('fabric-app'),
    customElements.whenDefined('fabric-nav'),
    customElements.whenDefined('fabric-header'),
    customElements.whenDefined('fabric-breadcrumb'),
    customElements.whenDefined('fabric-status-badge'),
    customElements.whenDefined('fabric-debug-panel'),
  ]);

  console.info('[Fabric] Components defined, setting up router...');

  // First-run redirect: if the system hasn't completed onboarding, navigate to /onboarding
  const skipRedirectPaths = ['/onboarding', '/login', '/invite'];
  if (!skipRedirectPaths.includes(window.location.pathname)) {
    try {
      const statusRes = await fetch('/api/v1/system/status', { credentials: 'include' });
      if (statusRes.ok) {
        const status = await statusRes.json();
        if (!status.complete) {
          sessionStorage.setItem('onboardingStatus', JSON.stringify(status));
          window.history.replaceState({}, '', '/onboarding');
        }
      }
    } catch {
      // System status endpoint unavailable (non-workstation mode) — skip redirect
    }
  }

  // Render the initial page based on current URL
  await renderRoute(window.location.pathname);

  // Setup client-side router for navigation
  setupRouter();

  // Disconnect SSE on page unload
  window.addEventListener('beforeunload', () => {
    stateManager.disconnect();
  });

  console.info('[Fabric] Client initialization complete');
}

/**
 * Retrieves initial page data from SSR-injected script tag
 */
function getInitialData(): PageData | null {
  const script = document.getElementById('__FABRIC_DATA__');
  if (!script) {
    console.warn('[Fabric] No initial data found');
    return null;
  }

  try {
    return JSON.parse(script.textContent || '{}') as PageData;
  } catch (e) {
    console.error('[Fabric] Failed to parse initial data:', e);
    return null;
  }
}

/** Fallback route for unmatched paths */
const NOT_FOUND_ROUTE: RouteConfig = {
  pattern: /./,
  tag: 'fabric-page-404',
  load: () => import('../components/pages/not-found.js'),
};

/**
 * Resolves a URL path to a route configuration
 */
function resolveRoute(path: string): RouteConfig {
  for (const route of ROUTES) {
    if (route.pattern.test(path)) {
      return route;
    }
  }
  return NOT_FOUND_ROUTE;
}

/**
 * Determines which shell type a route tag requires.
 */
type ShellType = 'standalone' | 'profile' | 'app';

function getShellType(tag: string): ShellType {
  if (STANDALONE_ROUTES.has(tag)) return 'standalone';
  if (PROFILE_ROUTES.has(tag)) return 'profile';
  return 'app';
}

/** Cached shell element and its type, reused across navigations */
let activeShell: { type: ShellType; element: HTMLElement } | null = null;

/** Navigation counter to cancel stale renders when rapid navigations occur */
let navigationId = 0;

/**
 * Renders the page component for the given path into #app.
 * Lazily imports the page module before creating the element.
 * Reuses the shell element (sidebar, header, etc.) when possible
 * to avoid full-page redraws on navigation.
 */
async function renderRoute(path: string): Promise<void> {
  const appContainer = document.getElementById('app');
  if (!appContainer) return;

  // Strip query string and hash for route matching
  const pathname = path.split('?')[0].split('#')[0];
  const route = resolveRoute(pathname);
  const tag = route.tag;

  // Build page data with current user context for page components.
  // Include SSR-prefetched data on the initial render so page components
  // can skip redundant API fetches.
  const hasSsrData = ssrPageData && ssrPageData.path === path && ssrPageData.data;
  const pageData: PageData = {
    path,
    title: 'Fabric',
    user: currentUser || undefined,
    data: hasSsrData ? ssrPageData!.data : undefined,
  };
  // Consume SSR data so it is not reused on subsequent client-side navigations.
  if (hasSsrData) {
    ssrPageData = null;
  }

  // Block non-admin users from admin-only routes
  if (ADMIN_ROUTES.has(tag) && currentUser?.role !== 'admin') {
    navigateTo('/');
    return;
  }

  const shellType = getShellType(tag);

  // Lazy-load the page component module (and profile shell if needed).
  // The import registers the custom element as a side effect.
  const thisNav = ++navigationId;
  const loads: Promise<unknown>[] = [route.load()];
  if (shellType === 'profile' && !customElements.get('fabric-profile-shell')) {
    loads.push(
      import('../components/profile/profile-shell.js'),
      import('../components/profile/profile-nav.js'),
    );
  }
  await Promise.all(loads);

  // If another navigation started while we were loading, abort this render
  if (thisNav !== navigationId) return;

  // If the shell type changed, tear down and rebuild
  if (activeShell && activeShell.type !== shellType) {
    appContainer.innerHTML = '';
    activeShell = null;
  }

  if (shellType === 'standalone') {
    // Standalone pages render without a persistent shell
    appContainer.innerHTML = '';
    activeShell = null;
    const page = document.createElement(tag);
    appContainer.appendChild(page);
    setDocumentTitle(tag === 'fabric-login-page' ? 'Login' : tag === 'fabric-page-invite' ? 'Invite' : 'Page Not Found');
  } else if (activeShell) {
    // Reuse existing shell — just update properties and swap page content
    const shell = activeShell.element as HTMLElement & {
      currentPath: string;
      user: User | null;
    };
    shell.currentPath = path;
    shell.user = currentUser;

    // Replace only the page content inside the shell
    const oldPage = shell.querySelector('[data-fabric-page]');
    if (oldPage) oldPage.remove();

    const page = document.createElement(tag) as HTMLElement & { pageData: PageData };
    page.pageData = pageData;
    page.setAttribute('data-fabric-page', '');
    shell.appendChild(page);
  } else {
    // Create the shell for the first time — clear any SSR-rendered content
    appContainer.innerHTML = '';
    const shellTag = shellType === 'profile' ? 'fabric-profile-shell' : 'fabric-app';
    const shell = document.createElement(shellTag) as HTMLElement & {
      currentPath: string;
      user: User | null;
    };
    shell.currentPath = path;
    shell.user = currentUser;

    const page = document.createElement(tag) as HTMLElement & { pageData: PageData };
    page.pageData = pageData;
    page.setAttribute('data-fabric-page', '');
    shell.appendChild(page);
    appContainer.appendChild(shell);

    activeShell = { type: shellType, element: shell };
  }
}

/**
 * Sets up the client-side router for navigation
 */
function setupRouter(): void {
  // Add click handlers for client-side navigation.
  // Use the composed event path to find anchors inside shadow DOMs,
  // since target.closest('a') cannot cross shadow boundaries.
  document.addEventListener('click', (e: MouseEvent) => {
    const path = e.composedPath();
    let anchor: HTMLAnchorElement | null = null;
    for (const el of path) {
      if (el instanceof HTMLAnchorElement) {
        anchor = el;
        break;
      }
    }

    if (!anchor) return;

    const href = anchor.getAttribute('href');
    if (!href) return;

    // Skip external links
    if (href.startsWith('http') || href.startsWith('//')) return;

    // Skip special links
    if (href.startsWith('javascript:')) return;
    if (href.startsWith('#')) return;

    // Skip links that should trigger full page loads
    if (href.startsWith('/api/')) return;
    if (href.startsWith('/auth/')) return;
    if (href.startsWith('/events')) return;

    // Handle client-side navigation
    e.preventDefault();
    navigateTo(href);
  });

  // Handle nav-click events from shadow DOM components (e.g. sidebar nav)
  // These events use composed:true to cross shadow boundaries.
  document.addEventListener('nav-click', ((e: CustomEvent<{ path: string }>) => {
    const path = e.detail?.path;
    if (path) {
      navigateTo(path);
    }
  }) as EventListener);

  // Handle browser back/forward
  window.addEventListener('popstate', () => {
    void renderRoute(window.location.pathname);
  });
}

/**
 * Navigates to a new path using the History API
 */
function navigateTo(path: string): void {
  if (path === window.location.pathname) return;

  window.history.pushState({}, '', path);
  void renderRoute(path);
}

// Initialize when DOM is ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', () => {
    void init();
  });
} else {
  void init();
}

// Export for use in components and tests
export { getInitialData, navigateTo, stateManager };
