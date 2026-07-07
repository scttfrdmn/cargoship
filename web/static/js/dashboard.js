// CargoShip Dashboard JavaScript
class CargoShipDashboard {
    constructor() {
        this.apiBase = '/api/v1';
        // Match the page's scheme so the socket is secure (wss) whenever the
        // dashboard is served over HTTPS.
        const wsScheme = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        this.wsUrl = `${wsScheme}//${window.location.host}/api/v1/ws`;
        this.ws = null;
        this.agents = [];
        this.jobs = [];
        this.metrics = {};
        this.refreshInterval = null;
        this.wsReconnectTimeout = null;
        
        this.init();
    }
    
    init() {
        this.setupEventListeners();
        this.connectWebSocket();
        this.startAutoRefresh();
        this.loadInitialData();
    }
    
    setupEventListeners() {
        // Refresh button
        document.getElementById('refresh-btn').addEventListener('click', () => {
            this.loadAllData();
            this.showToast('Data refreshed', 'success');
        });
        
        // Job filter
        document.getElementById('job-filter').addEventListener('change', (e) => {
            this.filterJobs(e.target.value);
        });
        
        // Modal close handlers
        document.addEventListener('click', (e) => {
            if (e.target.classList.contains('modal')) {
                this.closeModal(e.target.id);
            }
        });
        
        // Disconnect agent handler
        document.getElementById('disconnect-agent-btn').addEventListener('click', () => {
            this.disconnectAgent();
        });
    }
    
    // WebSocket Connection
    connectWebSocket() {
        try {
            this.ws = new WebSocket(this.wsUrl);
            
            this.ws.onopen = () => {
                console.log('WebSocket connected');
                this.updateConnectionStatus(true);
                this.clearReconnectTimeout();
            };
            
            this.ws.onmessage = (event) => {
                const message = JSON.parse(event.data);
                this.handleWebSocketMessage(message);
            };
            
            this.ws.onclose = () => {
                console.log('WebSocket disconnected');
                this.updateConnectionStatus(false);
                this.scheduleReconnect();
            };
            
            this.ws.onerror = (error) => {
                console.error('WebSocket error:', error);
                this.updateConnectionStatus(false);
            };
        } catch (error) {
            console.error('Failed to connect WebSocket:', error);
            this.updateConnectionStatus(false);
            this.scheduleReconnect();
        }
    }
    
    scheduleReconnect() {
        this.clearReconnectTimeout();
        this.wsReconnectTimeout = setTimeout(() => {
            console.log('Attempting WebSocket reconnection...');
            this.connectWebSocket();
        }, 5000);
    }
    
    clearReconnectTimeout() {
        if (this.wsReconnectTimeout) {
            clearTimeout(this.wsReconnectTimeout);
            this.wsReconnectTimeout = null;
        }
    }
    
    handleWebSocketMessage(message) {
        switch (message.type) {
            case 'agent_connected':
            case 'agent_disconnected':
            case 'agent_updated':
                this.loadAgents();
                this.loadMetrics();
                break;
            case 'job_started':
            case 'job_completed':
            case 'job_failed':
            case 'job_cancelled':
                this.loadJobs();
                this.loadMetrics();
                break;
            default:
                console.log('Unknown WebSocket message type:', message.type);
        }
    }
    
    updateConnectionStatus(connected) {
        const statusDot = document.getElementById('connection-status');
        const statusText = document.getElementById('connection-text');
        
        if (connected) {
            statusDot.className = 'status-dot connected';
            statusText.textContent = 'Connected';
        } else {
            statusDot.className = 'status-dot disconnected';
            statusText.textContent = 'Disconnected';
        }
    }
    
    // API Calls
    async apiCall(endpoint, options = {}) {
        try {
            const response = await fetch(`${this.apiBase}${endpoint}`, {
                headers: {
                    'Content-Type': 'application/json',
                    ...options.headers
                },
                ...options
            });
            
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            return await response.json();
        } catch (error) {
            // nosemgrep: javascript.lang.security.audit.unsafe-formatstring.unsafe-formatstring -- template literal, not a util.format format string; endpoint is an internal constant
            console.error(`API call failed (${endpoint}):`, error);
            this.showToast(`Failed to ${endpoint}: ${error.message}`, 'error');
            throw error;
        }
    }
    
    // Data Loading
    async loadInitialData() {
        await this.loadAllData();
    }
    
    async loadAllData() {
        await Promise.all([
            this.loadAgents(),
            this.loadJobs(),
            this.loadMetrics()
        ]);
    }
    
    async loadAgents() {
        try {
            this.agents = await this.apiCall('/agents');
            this.renderAgentsTable();
        } catch (error) {
            console.error('Failed to load agents:', error);
        }
    }
    
    async loadJobs() {
        try {
            this.jobs = await this.apiCall('/jobs');
            this.renderJobsTable();
        } catch (error) {
            console.error('Failed to load jobs:', error);
        }
    }
    
    async loadMetrics() {
        try {
            this.metrics = await this.apiCall('/metrics');
            this.renderMetrics();
        } catch (error) {
            console.error('Failed to load metrics:', error);
        }
    }
    
    // Rendering
    renderMetrics() {
        document.getElementById('total-agents').textContent = this.metrics.total_agents || 0;
        document.getElementById('active-jobs').textContent = this.metrics.active_jobs || 0;
        document.getElementById('completed-jobs').textContent = this.metrics.completed_jobs || 0;
        document.getElementById('throughput').textContent = this.metrics.total_throughput || '0 MB/s';
    }
    
    renderAgentsTable() {
        const tbody = document.getElementById('agents-tbody');
        const emptyState = document.getElementById('agents-empty');
        
        if (this.agents.length === 0) {
            tbody.innerHTML = '';
            emptyState.style.display = 'block';
            return;
        }
        
        emptyState.style.display = 'none';
        
        tbody.innerHTML = this.agents.map(agent => `
            <tr>
                <td>
                    <span class="status-badge ${agent.connected ? 'connected' : 'disconnected'}">
                        ${agent.connected ? '●' : '○'} ${agent.status}
                    </span>
                </td>
                <td class="font-mono">${this.escapeHtml(agent.name)}</td>
                <td class="font-mono text-secondary">${this.escapeHtml(agent.id.substring(0, 8))}...</td>
                <td class="font-mono">${this.escapeHtml(agent.endpoint)}</td>
                <td>${agent.jobs}</td>
                <td>
                    <div class="progress-bar">
                        <div class="progress-fill" style="width: ${agent.progress}%"></div>
                    </div>
                    <span class="text-sm text-secondary">${agent.progress.toFixed(1)}%</span>
                </td>
                <td>${this.escapeHtml(agent.throughput)}</td>
                <td>${this.formatTimestamp(agent.last_seen)}</td>
                <td>
                    <button class="btn btn-sm btn-secondary" onclick="dashboard.showAgentDetails('${agent.id}')">
                        👁️ View
                    </button>
                </td>
            </tr>
        `).join('');
    }
    
    renderJobsTable(filteredJobs = null) {
        const jobs = filteredJobs || this.jobs;
        const tbody = document.getElementById('jobs-tbody');
        const emptyState = document.getElementById('jobs-empty');
        
        if (jobs.length === 0) {
            tbody.innerHTML = '';
            emptyState.style.display = 'block';
            return;
        }
        
        emptyState.style.display = 'none';
        
        tbody.innerHTML = jobs.map(job => `
            <tr>
                <td>
                    <span class="status-badge ${job.status}">
                        ${this.getJobStatusIcon(job.status)} ${job.status}
                    </span>
                </td>
                <td class="font-mono">${this.escapeHtml(job.id.substring(0, 8))}...</td>
                <td class="font-mono">${this.escapeHtml(job.agent_id.substring(0, 8))}...</td>
                <td>${this.escapeHtml(job.type)}</td>
                <td class="truncate" style="max-width: 200px;" title="${this.escapeHtml(job.path)}">
                    ${this.escapeHtml(job.path)}
                </td>
                <td>
                    <div class="progress-bar">
                        <div class="progress-fill" style="width: ${job.progress}%"></div>
                    </div>
                    <span class="text-sm text-secondary">${job.progress.toFixed(1)}%</span>
                </td>
                <td>${this.escapeHtml(job.size)}</td>
                <td>${this.escapeHtml(job.rate)}</td>
                <td>${this.formatTimestamp(job.start_time)}</td>
                <td>
                    ${job.status === 'running' || job.status === 'pending' ? 
                        `<button class="btn btn-sm btn-danger" onclick="dashboard.cancelJob('${job.id}')">❌ Cancel</button>` :
                        `<span class="text-secondary">—</span>`
                    }
                </td>
            </tr>
        `).join('');
    }
    
    // Filtering
    filterJobs(status) {
        if (!status) {
            this.renderJobsTable();
            return;
        }
        
        const filtered = this.jobs.filter(job => job.status === status);
        this.renderJobsTable(filtered);
    }
    
    // Agent Actions
    async showAgentDetails(agentId) {
        const agent = this.agents.find(a => a.id === agentId);
        if (!agent) return;
        
        document.getElementById('modal-agent-name').textContent = agent.name;
        document.getElementById('modal-agent-id').textContent = agent.id;
        document.getElementById('modal-agent-status').textContent = agent.status;
        document.getElementById('modal-agent-endpoint').textContent = agent.endpoint;
        document.getElementById('modal-agent-lastseen').textContent = this.formatTimestamp(agent.last_seen);
        
        const progressBar = document.getElementById('modal-agent-progress');
        progressBar.style.width = `${agent.progress}%`;
        
        // Store current agent ID for disconnect action
        document.getElementById('disconnect-agent-btn').dataset.agentId = agentId;
        
        this.showModal('agent-modal');
    }
    
    async disconnectAgent() {
        const agentId = document.getElementById('disconnect-agent-btn').dataset.agentId;
        if (!agentId) return;
        
        try {
            await this.apiCall(`/agents/${agentId}/disconnect`, { method: 'POST' });
            this.showToast('Agent disconnected successfully', 'success');
            this.closeModal('agent-modal');
            this.loadAgents();
            this.loadMetrics();
        } catch (error) {
            this.showToast('Failed to disconnect agent', 'error');
        }
    }
    
    // Job Actions
    async cancelJob(jobId) {
        if (!confirm('Are you sure you want to cancel this job?')) return;
        
        try {
            await this.apiCall(`/jobs/${jobId}/cancel`, { method: 'POST' });
            this.showToast('Job cancelled successfully', 'success');
            this.loadJobs();
            this.loadMetrics();
        } catch (error) {
            this.showToast('Failed to cancel job', 'error');
        }
    }
    
    // Modal Management
    showModal(modalId) {
        const modal = document.getElementById(modalId);
        modal.classList.add('show');
    }
    
    closeModal(modalId) {
        const modal = document.getElementById(modalId);
        modal.classList.remove('show');
    }
    
    // Toast Notifications
    showToast(message, type = 'success') {
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.innerHTML = `
            <div style="display: flex; align-items: center; justify-content: space-between;">
                <span>${this.escapeHtml(message)}</span>
                <button onclick="this.parentElement.parentElement.remove()" style="background: none; border: none; font-size: 1.2rem; cursor: pointer; margin-left: 1rem;">&times;</button>
            </div>
        `;
        
        document.getElementById('toast-container').appendChild(toast);
        
        // Auto remove after 5 seconds
        setTimeout(() => {
            if (toast.parentElement) {
                toast.remove();
            }
        }, 5000);
    }
    
    // Auto Refresh
    startAutoRefresh() {
        this.refreshInterval = setInterval(() => {
            this.loadAllData();
        }, 30000); // Refresh every 30 seconds
    }
    
    stopAutoRefresh() {
        if (this.refreshInterval) {
            clearInterval(this.refreshInterval);
            this.refreshInterval = null;
        }
    }
    
    // Utility Functions
    formatTimestamp(timestamp) {
        if (!timestamp) return '—';
        
        const date = new Date(timestamp);
        const now = new Date();
        const diff = now - date;
        
        if (diff < 60000) { // Less than 1 minute
            return 'Just now';
        } else if (diff < 3600000) { // Less than 1 hour
            return `${Math.floor(diff / 60000)}m ago`;
        } else if (diff < 86400000) { // Less than 1 day
            return `${Math.floor(diff / 3600000)}h ago`;
        } else {
            return date.toLocaleDateString();
        }
    }
    
    getJobStatusIcon(status) {
        switch (status) {
            case 'running': return '⚡';
            case 'pending': return '⏳';
            case 'completed': return '✅';
            case 'failed': return '❌';
            default: return '●';
        }
    }
    
    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }
    
    // Cleanup
    destroy() {
        this.stopAutoRefresh();
        this.clearReconnectTimeout();
        if (this.ws) {
            this.ws.close();
        }
    }
}

// Global functions for HTML event handlers
function closeModal(modalId) {
    dashboard.closeModal(modalId);
}

// Initialize dashboard when page loads
let dashboard;
document.addEventListener('DOMContentLoaded', () => {
    dashboard = new CargoShipDashboard();
});

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
    if (dashboard) {
        dashboard.destroy();
    }
});