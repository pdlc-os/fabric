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
 * Shared Components Exports
 *
 * Re-exports all shared Lit components
 */

export { FabricNav } from './nav.js';
export { FabricHeader } from './header.js';
export { FabricBreadcrumb } from './breadcrumb.js';
export { FabricStatusBadge } from './status-badge.js';
export type { StatusType } from './status-badge.js';
export { FabricDebugPanel } from './debug-panel.js';
export { FabricEnvVarList } from './env-var-list.js';
export { FabricSecretList } from './secret-list.js';
export { FabricNotificationTray } from './notification-tray.js';
export { FabricInboxTray } from './inbox-tray.js';
export { FabricViewToggle } from './view-toggle.js';
export type { ViewMode } from './view-toggle.js';
export { resourceStyles, listPageStyles } from './resource-styles.js';
export { FabricJsonBrowser } from './json-browser.js';
export { FabricAgentLogViewer } from './agent-log-viewer.js';
export { FabricSharedDirList } from './shared-dir-list.js';
export { FabricFileBrowser } from './file-browser.js';
export type { FileEntry, FileListResult, FileBrowserDataSource } from './file-browser.js';
export { WorkspaceFileBrowserDataSource, SharedDirFileBrowserDataSource, TemplateFileBrowserDataSource } from './file-browser.js';
export { FabricCodeEditor, getLanguageFromPath } from './code-editor.js';
export { FabricFileEditor, WorkspaceFileEditorDataSource, SharedDirFileEditorDataSource, TemplateFileEditorDataSource } from './file-editor.js';
export type { FileEditorDataSource, FileContentResponse } from './file-editor.js';
export { FabricGCPServiceAccountList } from './gcp-service-account-list.js';
export { FabricSubscriptionManager } from './subscription-manager.js';
export { FabricGitRemoteDisplay } from './git-remote-display.js';
export { FabricHashDisplay } from './hash-display.js';
