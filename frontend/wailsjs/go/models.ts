export namespace session {
	
	export class AgentDTO {
	    id: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	    }
	}
	export class SessionDTO {
	    id: number;
	    worktreeId: number;
	    adapterId: string;
	    label: string;
	    title: string;
	    status: string;
	    cwdPath: string;
	    peerId: string;
	    peerCapable: boolean;
	    peerSummary: string;
	    outstandingPeerCount: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.worktreeId = source["worktreeId"];
	        this.adapterId = source["adapterId"];
	        this.label = source["label"];
	        this.title = source["title"];
	        this.status = source["status"];
	        this.cwdPath = source["cwdPath"];
	        this.peerId = source["peerId"];
	        this.peerCapable = source["peerCapable"];
	        this.peerSummary = source["peerSummary"];
	        this.outstandingPeerCount = source["outstandingPeerCount"];
	    }
	}
	export class WorktreeDTO {
	    id: number;
	    repoId: number;
	    name: string;
	    rootPath: string;
	    gitBranch: string;
	    isPrimary: boolean;
	    sessions: SessionDTO[];
	
	    static createFrom(source: any = {}) {
	        return new WorktreeDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.repoId = source["repoId"];
	        this.name = source["name"];
	        this.rootPath = source["rootPath"];
	        this.gitBranch = source["gitBranch"];
	        this.isPrimary = source["isPrimary"];
	        this.sessions = this.convertValues(source["sessions"], SessionDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RepoDTO {
	    id: number;
	    name: string;
	    rootPath: string;
	    gitCommonDir: string;
	    isGitRepo: boolean;
	    persistenceKey: string;
	    worktrees: WorktreeDTO[];
	
	    static createFrom(source: any = {}) {
	        return new RepoDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.rootPath = source["rootPath"];
	        this.gitCommonDir = source["gitCommonDir"];
	        this.isGitRepo = source["isGitRepo"];
	        this.persistenceKey = source["persistenceKey"];
	        this.worktrees = this.convertValues(source["worktrees"], WorktreeDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AppSnapshot {
	    repos: RepoDTO[];
	    activeRepoId: number;
	    activeWorktreeId: number;
	    activeSessionId: number;
	    availableAgents: AgentDTO[];
	    lastUsedAgentId: string;
	
	    static createFrom(source: any = {}) {
	        return new AppSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repos = this.convertValues(source["repos"], RepoDTO);
	        this.activeRepoId = source["activeRepoId"];
	        this.activeWorktreeId = source["activeWorktreeId"];
	        this.activeSessionId = source["activeSessionId"];
	        this.availableAgents = this.convertValues(source["availableAgents"], AgentDTO);
	        this.lastUsedAgentId = source["lastUsedAgentId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PeerMessageDTO {
	    id: number;
	    fromPeerId: string;
	    toPeerId: string;
	    fromLabel: string;
	    fromTitle: string;
	    replyToId: number;
	    body: string;
	    status: string;
	    createdAtUnixMs: number;
	    noticedAtUnixMs: number;
	    readAtUnixMs: number;
	    ackedAtUnixMs: number;
	    failedAtUnixMs: number;
	    failureReason: string;
	
	    static createFrom(source: any = {}) {
	        return new PeerMessageDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.fromPeerId = source["fromPeerId"];
	        this.toPeerId = source["toPeerId"];
	        this.fromLabel = source["fromLabel"];
	        this.fromTitle = source["fromTitle"];
	        this.replyToId = source["replyToId"];
	        this.body = source["body"];
	        this.status = source["status"];
	        this.createdAtUnixMs = source["createdAtUnixMs"];
	        this.noticedAtUnixMs = source["noticedAtUnixMs"];
	        this.readAtUnixMs = source["readAtUnixMs"];
	        this.ackedAtUnixMs = source["ackedAtUnixMs"];
	        this.failedAtUnixMs = source["failedAtUnixMs"];
	        this.failureReason = source["failureReason"];
	    }
	}
	export class PeerDTO {
	    peerId: string;
	    sessionId: number;
	    adapterId: string;
	    adapterFamily: string;
	    label: string;
	    title: string;
	    summary: string;
	    repoKey: string;
	    worktreeRootPath: string;
	    outstandingCount: number;
	    unreadCount: number;
	    isSelf: boolean;
	    lastHeartbeatUnixMs: number;
	
	    static createFrom(source: any = {}) {
	        return new PeerDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.peerId = source["peerId"];
	        this.sessionId = source["sessionId"];
	        this.adapterId = source["adapterId"];
	        this.adapterFamily = source["adapterFamily"];
	        this.label = source["label"];
	        this.title = source["title"];
	        this.summary = source["summary"];
	        this.repoKey = source["repoKey"];
	        this.worktreeRootPath = source["worktreeRootPath"];
	        this.outstandingCount = source["outstandingCount"];
	        this.unreadCount = source["unreadCount"];
	        this.isSelf = source["isSelf"];
	        this.lastHeartbeatUnixMs = source["lastHeartbeatUnixMs"];
	    }
	}
	export class PeerStateDTO {
	    peers: PeerDTO[];
	    messages: PeerMessageDTO[];
	
	    static createFrom(source: any = {}) {
	        return new PeerStateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.peers = this.convertValues(source["peers"], PeerDTO);
	        this.messages = this.convertValues(source["messages"], PeerMessageDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UIStateDTO {
	    sidebarOpen: boolean;
	    sidebarWidth: number;
	    diffPanelOpen: boolean;
	    diffPanelWidth: number;
	    terminalFontSize: number;
	    utilityPanelTab: string;
	    collapsedRepoKeys: string[];
	
	    static createFrom(source: any = {}) {
	        return new UIStateDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sidebarOpen = source["sidebarOpen"];
	        this.sidebarWidth = source["sidebarWidth"];
	        this.diffPanelOpen = source["diffPanelOpen"];
	        this.diffPanelWidth = source["diffPanelWidth"];
	        this.terminalFontSize = source["terminalFontSize"];
	        this.utilityPanelTab = source["utilityPanelTab"];
	        this.collapsedRepoKeys = source["collapsedRepoKeys"];
	    }
	}
	export class BootstrapResult {
	    snapshot: AppSnapshot;
	    uiState: UIStateDTO;
	    restoreNotice: string;
	    peerState: PeerStateDTO;
	
	    static createFrom(source: any = {}) {
	        return new BootstrapResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snapshot = this.convertValues(source["snapshot"], AppSnapshot);
	        this.uiState = this.convertValues(source["uiState"], UIStateDTO);
	        this.restoreNotice = source["restoreNotice"];
	        this.peerState = this.convertValues(source["peerState"], PeerStateDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FileDiff {
	    worktreeId: number;
	    path: string;
	    staged: boolean;
	    patch: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new FileDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.worktreeId = source["worktreeId"];
	        this.path = source["path"];
	        this.staged = source["staged"];
	        this.patch = source["patch"];
	        this.message = source["message"];
	    }
	}
	export class GitFileChange {
	    path: string;
	    status: string;
	    added: number;
	    removed: number;
	
	    static createFrom(source: any = {}) {
	        return new GitFileChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.status = source["status"];
	        this.added = source["added"];
	        this.removed = source["removed"];
	    }
	}
	
	
	
	
	
	
	export class WorkspaceChoice {
	    rootPath: string;
	    name: string;
	    gitBranch: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceChoice(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rootPath = source["rootPath"];
	        this.name = source["name"];
	        this.gitBranch = source["gitBranch"];
	    }
	}
	export class WorktreeCreateRequest {
	    mode: string;
	    branchName: string;
	    sourceRef: string;
	    path: string;
	    agentId: string;
	
	    static createFrom(source: any = {}) {
	        return new WorktreeCreateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.branchName = source["branchName"];
	        this.sourceRef = source["sourceRef"];
	        this.path = source["path"];
	        this.agentId = source["agentId"];
	    }
	}
	
	export class WorktreeDiff {
	    worktreeId: number;
	    rootPath: string;
	    gitBranch: string;
	    isGitRepo: boolean;
	    staged: GitFileChange[];
	    unstaged: GitFileChange[];
	    untracked: string[];
	    stagedPatch: string;
	    unstagedPatch: string;
	
	    static createFrom(source: any = {}) {
	        return new WorktreeDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.worktreeId = source["worktreeId"];
	        this.rootPath = source["rootPath"];
	        this.gitBranch = source["gitBranch"];
	        this.isGitRepo = source["isGitRepo"];
	        this.staged = this.convertValues(source["staged"], GitFileChange);
	        this.unstaged = this.convertValues(source["unstaged"], GitFileChange);
	        this.untracked = source["untracked"];
	        this.stagedPatch = source["stagedPatch"];
	        this.unstagedPatch = source["unstagedPatch"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WorktreeEntry {
	    name: string;
	    path: string;
	    kind: string;
	    expandable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new WorktreeEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.kind = source["kind"];
	        this.expandable = source["expandable"];
	    }
	}
	export class WorktreeFile {
	    path: string;
	    content: string;
	    versionToken: string;
	
	    static createFrom(source: any = {}) {
	        return new WorktreeFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	        this.versionToken = source["versionToken"];
	    }
	}

}

