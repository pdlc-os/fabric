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
 * Component exports
 *
 * Re-exports all Lit components for easy importing
 */

// App shell
export { FabricApp } from './app-shell.js';

// Shared components
export { FabricNav, FabricHeader, FabricBreadcrumb, FabricStatusBadge, FabricViewToggle } from './shared/index.js';
export type { StatusType } from './shared/index.js';
export type { ViewMode } from './shared/index.js';

// Pages
export { FabricPageHome } from './pages/home.js';
export { FabricPageProjects } from './pages/projects.js';
export { FabricPageProjectDetail } from './pages/project-detail.js';
export { FabricPageAgents } from './pages/agents.js';
export { FabricPageAgentDetail } from './pages/agent-detail.js';
export { FabricPageAgentCreate } from './pages/agent-create.js';
export { FabricPageProjectCreate } from './pages/project-create.js';
export { FabricPageProjectSettings } from './pages/project-settings.js';
export { FabricPageProjectSchedules } from './pages/project-schedules.js';
export { FabricPageBrokers } from './pages/brokers.js';
export { FabricPageAdminUsers } from './pages/admin-users.js';
export { FabricPageAdminGroups } from './pages/admin-groups.js';
export { FabricPageProfileEnvVars } from './pages/profile-env-vars.js';
export { FabricPageProfileSecrets } from './pages/profile-secrets.js';
export { FabricPageSettings } from './pages/settings.js';
export { FabricPage404 } from './pages/not-found.js';
export { FabricLoginPage } from './pages/login.js';

// Profile shell
export { FabricProfileShell } from './profile/profile-shell.js';
export { FabricProfileNav } from './profile/profile-nav.js';
