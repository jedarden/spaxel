// Notification Settings Panel for Spaxel Dashboard
// Provides UI for configuring push notification channels, quiet hours, and batching

export class NotificationSettings {
	constructor(api) {
		this.api = api;
		this.container = null;
		this.channels = [];
		this.quietHours = null;
		this.batching = null;
	}

	async init() {
		this.container = document.getElementById('settings-content');
		if (!this.container) {
			console.error('Settings content container not found');
			return;
		}

		await this.loadConfig();
		this.render();
		this.attachListeners();
	}

	async loadConfig() {
		try {
			const [channelsRes, quietRes, batchRes] = await Promise.all([
				fetch('/api/notifications/channels'),
				fetch('/api/notifications/quiet-hours'),
				fetch('/api/notifications/batching')
			]);

			if (channelsRes.ok) {
				const data = await channelsRes.json();
				this.channels = data.channels || [];
			}

			if (quietRes.ok) {
				this.quietHours = await quietRes.json();
			}

			if (batchRes.ok) {
				this.batching = await batchRes.json();
			}
		} catch (error) {
			console.error('Failed to load notification config:', error);
		}
	}

	render() {
		this.container.innerHTML = `
			<div class="notification-settings">
				<h2>Notification Settings</h2>

				<!-- Delivery Channels Section -->
				<section class="settings-section">
					<h3>Delivery Channels</h3>
					<p class="section-desc">Configure where to send push notifications. Multiple channels can be enabled simultaneously.</p>

					<div class="channels-list" id="channels-list">
						${this.renderChannelsList()}
					</div>

					<button class="btn btn-primary" id="add-channel-btn">
						<span class="icon">+</span> Add Channel
					</button>
				</section>

				<!-- Quiet Hours Section -->
				<section class="settings-section">
					<h3>Quiet Hours</h3>
					<p class="section-desc">Configure times when low-priority notifications are silenced.</p>

					<div class="quiet-hours-config">
						<label class="checkbox-label">
							<input type="checkbox" id="quiet-hours-enabled"
								${this.quietHours?.enabled ? 'checked' : ''}>
							Enable Quiet Hours
						</label>

						<div class="time-range">
							<div class="time-field">
								<label>From</label>
								<input type="time" id="quiet-hours-start"
									value="${this.formatTime(this.quietHours?.start_hour, this.quietHours?.start_min)}">
							</div>
							<div class="time-field">
								<label>To</label>
								<input type="time" id="quiet-hours-end"
									value="${this.formatTime(this.quietHours?.end_hour, this.quietHours?.end_min)}">
							</div>
						</div>

						<div class="days-selector">
							<label>Active Days</label>
							${this.renderDaySelector()}
						</div>

						<label class="checkbox-label">
							<input type="checkbox" id="morning-digest-enabled"
								${this.quietHours?.morning_digest ? 'checked' : ''}>
							Morning Digest (deliver queued events at wake time)
						</label>

						<div class="time-field">
							<label>Digest Time</label>
							<input type="time" id="digest-time"
								value="${this.formatTime(this.quietHours?.digest_hour, this.quietHours?.digest_min)}">
						</div>
					</div>
				</section>

				<!-- Smart Batching Section -->
				<section class="settings-section">
					<h3>Smart Batching</h3>
					<p class="section-desc">Combine multiple events into a single notification to reduce noise.</p>

					<div class="batching-config">
						<label class="checkbox-label">
							<input type="checkbox" id="batching-enabled"
								${this.batching?.enabled ? 'checked' : ''}>
							Enable Smart Batching
						</label>

						<div class="batch-window">
							<label>Batch Window (seconds)</label>
							<input type="number" id="batch-window" min="5" max="300" step="5"
								value="${this.batching?.batch_window_sec || 30}">
						</div>

						<div class="max-batch-size">
							<label>Max Batch Size</label>
							<input type="number" id="max-batch-size" min="1" max="20" step="1"
								value="${this.batching?.max_batch_size || 5}">
						</div>

						<label class="checkbox-label">
							<input type="checkbox" id="batch-low"
								${this.batching?.batch_low ? 'checked' : ''}>
							Batch Low Priority Events
						</label>

						<label class="checkbox-label">
							<input type="checkbox" id="batch-medium"
								${this.batching?.batch_medium ? 'checked' : ''}>
							Batch Medium Priority Events
						</label>
					</div>
				</section>

				<!-- Event Types Section -->
				<section class="settings-section">
					<h3>Event Types</h3>
					<p class="section-desc">Choose which types of events trigger notifications.</p>

					<div class="event-types">
						${this.renderEventTypeToggles()}
					</div>
				</section>

				<!-- Test Section -->
				<section class="settings-section">
					<h3>Test Notifications</h3>
					<p class="section-desc">Send a test notification to verify your configuration.</p>

					<button class="btn btn-secondary" id="test-notification-btn">
						<span class="icon">🔔</span> Send Test Notification
					</button>
				</section>
			</div>

			<!-- Add Channel Modal -->
			<div id="add-channel-modal" class="modal hidden">
				<div class="modal-content">
					<div class="modal-header">
						<h3>Add Notification Channel</h3>
						<button class="close-modal" id="close-modal-btn">×</button>
					</div>
					<div class="modal-body">
						<form id="add-channel-form">
							<div class="form-group">
								<label>Channel Type</label>
								<select id="channel-type" required>
									<option value="">Select type...</option>
									<option value="ntfy">Ntfy</option>
									<option value="pushover">Pushover</option>
									<option value="gotify">Gotify</option>
									<option value="webhook">Webhook</option>
								</select>
							</div>

							<div class="form-group" id="channel-id-group">
								<label>Channel ID</label>
								<input type="text" id="channel-id" placeholder="e.g., my-ntfy" required>
							</div>

							<div class="form-group" id="channel-url-group">
								<label>Server URL</label>
								<input type="url" id="channel-url" placeholder="https://ntfy.sh/my-topic">
							</div>

							<div class="form-group" id="channel-token-group">
								<label>Token / API Key</label>
								<input type="text" id="channel-token" placeholder="Your token or API key">
							</div>

							<div class="form-group" id="channel-user-group">
								<label>User Key (Pushover)</label>
								<input type="text" id="channel-user" placeholder="Your Pushover user key">
							</div>

							<div class="form-group" id="channel-auth-group">
								<label>Authentication (optional)</label>
								<div class="auth-fields">
									<input type="text" id="channel-username" placeholder="Username">
									<input type="password" id="channel-password" placeholder="Password">
								</div>
							</div>

							<div class="event-types-selector">
								<label>Enabled Events</label>
								<div id="modal-event-types">
									${this.renderModalEventTypeToggles()}
								</div>
							</div>

							<div class="form-actions">
								<button type="submit" class="btn btn-primary">Add Channel</button>
								<button type="button" class="btn btn-secondary" id="cancel-add-btn">Cancel</button>
							</div>
						</form>
					</div>
				</div>
			</div>
		`;
	}

	renderChannelsList() {
		if (!this.channels || this.channels.length === 0) {
			return '<p class="empty-state">No notification channels configured.</p>';
		}

		return this.channels.map(channel => `
			<div class="channel-card" data-id="${channel.id}">
				<div class="channel-header">
					<span class="channel-type">${this.getChannelTypeLabel(channel.type)}</span>
					<div class="channel-actions">
						<button class="btn-icon test-channel-btn" title="Test" data-id="${channel.id}">🔔</button>
						<button class="btn-icon delete-channel-btn" title="Delete" data-id="${channel.id}">🗑️</button>
					</div>
				</div>
				<div class="channel-details">
					${this.getChannelDetails(channel)}
				</div>
			</div>
		`).join('');
	}

	getChannelTypeLabel(type) {
		const labels = {
			'ntfy': 'Ntfy',
			'pushover': 'Pushover',
			'gotify': 'Gotify',
			'webhook': 'Webhook'
		};
		return labels[type] || type;
	}

	getChannelDetails(channel) {
		switch (channel.type) {
			case 'ntfy':
				return `<span class="channel-url">${channel.url || 'Not configured'}</span>`;
			case 'pushover':
				return `<span>User key configured</span>`;
			case 'gotify':
				return `<span class="channel-url">${channel.url || 'Not configured'}</span>`;
			case 'webhook':
				return `<span class="channel-url">${channel.url || 'Not configured'}</span>`;
			default:
				return '';
		}
	}

	renderDaySelector() {
		const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
		const mask = this.quietHours?.days_mask || 0x7F;

		return days.map((day, index) => {
			const isChecked = (mask & (1 << index)) !== 0;
			return `
				<label class="day-checkbox">
					<input type="checkbox" class="day-checkbox-input" data-day="${index}"
						${isChecked ? 'checked' : ''}>
					${day}
				</label>
			`;
		}).join('');
	}

	renderEventTypeToggles() {
		const eventTypes = [
			{ id: 'zone_enter', label: 'Zone Entry', description: 'When someone enters a zone' },
			{ id: 'zone_leave', label: 'Zone Leave', description: 'When someone leaves a zone' },
			{ id: 'zone_vacant', label: 'Zone Vacant', description: 'When a zone becomes empty' },
			{ id: 'fall_detected', label: 'Fall Detected', description: 'When a possible fall is detected' },
			{ id: 'fall_escalation', label: 'Fall Escalation', description: 'When fall is unacknowledged' },
			{ id: 'anomaly_alert', label: 'Anomaly Alert', description: 'Unusual activity detected' },
			{ id: 'node_offline', label: 'Node Offline', description: 'When a node goes offline' },
			{ id: 'sleep_summary', label: 'Sleep Summary', description: 'Daily sleep quality report' }
		];

		return eventTypes.map(event => `
			<div class="event-type-toggle">
				<label class="toggle-switch">
					<input type="checkbox" class="event-type-checkbox" data-event="${event.id}" checked>
					<span class="toggle-slider"></span>
				</label>
				<div class="event-type-info">
					<span class="event-type-label">${event.label}</span>
					<span class="event-type-desc">${event.description}</span>
				</div>
			</div>
		`).join('');
	}

	renderModalEventTypeToggles() {
		const eventTypes = [
			{ id: 'zone_enter', label: 'Zone Entry' },
			{ id: 'zone_leave', label: 'Zone Leave' },
			{ id: 'zone_vacant', label: 'Zone Vacant' },
			{ id: 'fall_detected', label: 'Fall Detected' },
			{ id: 'fall_escalation', label: 'Fall Escalation' },
			{ id: 'anomaly_alert', label: 'Anomaly Alert' },
			{ id: 'node_offline', label: 'Node Offline' },
			{ id: 'sleep_summary', label: 'Sleep Summary' }
		];

		return eventTypes.map(event => `
			<label class="checkbox-label">
				<input type="checkbox" class="modal-event-type-checkbox" data-event="${event.id}" checked>
				${event.label}
			</label>
		`).join('');
	}

	formatTime(hour, minute) {
		if (hour === undefined || minute === undefined) {
			return '';
		}
		const h = String(hour).padStart(2, '0');
		const m = String(minute).padStart(2, '0');
		return `${h}:${m}`;
	}

	attachListeners() {
		// Add channel button
		const addBtn = document.getElementById('add-channel-btn');
		if (addBtn) {
			addBtn.addEventListener('click', () => this.showAddChannelModal());
		}

		// Modal controls
		const closeModalBtn = document.getElementById('close-modal-btn');
		const cancelAddBtn = document.getElementById('cancel-add-btn');
		const modal = document.getElementById('add-channel-modal');

		if (closeModalBtn) {
			closeModalBtn.addEventListener('click', () => this.hideAddChannelModal());
		}
		if (cancelAddBtn) {
			cancelAddBtn.addEventListener('click', () => this.hideAddChannelModal());
		}
		if (modal) {
			modal.addEventListener('click', (e) => {
				if (e.target === modal) {
					this.hideAddChannelModal();
				}
			});
		}

		// Channel type selector - show/hide relevant fields
		const channelTypeSelect = document.getElementById('channel-type');
		if (channelTypeSelect) {
			channelTypeSelect.addEventListener('change', (e) => this.updateChannelTypeFields(e.target.value));
		}

		// Add channel form submission
		const addForm = document.getElementById('add-channel-form');
		if (addForm) {
			addForm.addEventListener('submit', (e) => this.handleAddChannel(e));
		}

		// Delete channel buttons
		document.querySelectorAll('.delete-channel-btn').forEach(btn => {
			btn.addEventListener('click', (e) => this.handleDeleteChannel(e.target.dataset.id));
		});

		// Test channel buttons
		document.querySelectorAll('.test-channel-btn').forEach(btn => {
			btn.addEventListener('click', (e) => this.handleTestChannel(e.target.dataset.id));
		});

		// Test notification button
		const testBtn = document.getElementById('test-notification-btn');
		if (testBtn) {
			testBtn.addEventListener('click', () => this.handleTestNotification());
		}

		// Quiet hours changes
		const quietEnabled = document.getElementById('quiet-hours-enabled');
		if (quietEnabled) {
			quietEnabled.addEventListener('change', () => this.saveQuietHours());
		}

		const quietStart = document.getElementById('quiet-hours-start');
		const quietEnd = document.getElementById('quiet-hours-end');
		if (quietStart) quietStart.addEventListener('change', () => this.saveQuietHours());
		if (quietEnd) quietEnd.addEventListener('change', () => this.saveQuietHours());

		// Day checkboxes
		document.querySelectorAll('.day-checkbox-input').forEach(cb => {
			cb.addEventListener('change', () => this.saveQuietHours());
		});

		// Morning digest
		const morningDigest = document.getElementById('morning-digest-enabled');
		const digestTime = document.getElementById('digest-time');
		if (morningDigest) morningDigest.addEventListener('change', () => this.saveQuietHours());
		if (digestTime) digestTime.addEventListener('change', () => this.saveQuietHours());

		// Batching changes
		const batchingEnabled = document.getElementById('batching-enabled');
		const batchWindow = document.getElementById('batch-window');
		const maxBatchSize = document.getElementById('max-batch-size');
		const batchLow = document.getElementById('batch-low');
		const batchMedium = document.getElementById('batch-medium');

		if (batchingEnabled) batchingEnabled.addEventListener('change', () => this.saveBatchingConfig());
		if (batchWindow) batchWindow.addEventListener('change', () => this.saveBatchingConfig());
		if (maxBatchSize) maxBatchSize.addEventListener('change', () => this.saveBatchingConfig());
		if (batchLow) batchLow.addEventListener('change', () => this.saveBatchingConfig());
		if (batchMedium) batchMedium.addEventListener('change', () => this.saveBatchingConfig());

		// Event type toggles
		document.querySelectorAll('.event-type-checkbox').forEach(cb => {
			cb.addEventListener('change', () => this.saveEventTypes());
		});
	}

	showAddChannelModal() {
		const modal = document.getElementById('add-channel-modal');
		if (modal) {
			modal.classList.remove('hidden');
		}
	}

	hideAddChannelModal() {
		const modal = document.getElementById('add-channel-modal');
		if (modal) {
			modal.classList.add('hidden');
		}
		// Reset form
		const form = document.getElementById('add-channel-form');
		if (form) form.reset();
	}

	updateChannelTypeFields(type) {
		// Show/hide fields based on channel type
		const urlGroup = document.getElementById('channel-url-group');
		const tokenGroup = document.getElementById('channel-token-group');
		const userGroup = document.getElementById('channel-user-group');
		const authGroup = document.getElementById('channel-auth-group');

		// Hide all first
		if (urlGroup) urlGroup.style.display = 'none';
		if (tokenGroup) tokenGroup.style.display = 'none';
		if (userGroup) userGroup.style.display = 'none';
		if (authGroup) authGroup.style.display = 'none';

		switch (type) {
			case 'ntfy':
			case 'gotify':
			case 'webhook':
				if (urlGroup) urlGroup.style.display = 'block';
				if (authGroup) authGroup.style.display = 'block';
				break;
			case 'pushover':
				if (tokenGroup) tokenGroup.style.display = 'block';
				if (userGroup) userGroup.style.display = 'block';
				break;
		}
	}

	async handleAddChannel(event) {
		event.preventDefault();

		const type = document.getElementById('channel-type').value;
		const id = document.getElementById('channel-id').value;
		const url = document.getElementById('channel-url').value;
		const token = document.getElementById('channel-token').value;
		const user = document.getElementById('channel-user').value;
		const username = document.getElementById('channel-username').value;
		const password = document.getElementById('channel-password').value;

		// Collect enabled event types
		const enabledTypes = {};
		document.querySelectorAll('.modal-event-type-checkbox:checked').forEach(cb => {
			enabledTypes[cb.dataset.event] = true;
		});

		try {
			const response = await fetch('/api/notifications/channels', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					id: id,
					type: type,
					url: url,
					token: token,
					user: user,
					username: username,
					password: password,
					enabled_types: enabledTypes
				})
			});

			if (response.ok) {
				this.hideAddChannelModal();
				await this.loadConfig();
				this.render();
				this.attachListeners();
				this.showSuccess('Channel added successfully');
			} else {
				const error = await response.json();
				this.showError(error.error || 'Failed to add channel');
			}
		} catch (error) {
			this.showError('Failed to add channel: ' + error.message);
		}
	}

	async handleDeleteChannel(id) {
		if (!confirm('Are you sure you want to delete this notification channel?')) {
			return;
		}

		try {
			const response = await fetch(`/api/notifications/channels/${id}`, {
				method: 'DELETE'
			});

			if (response.ok) {
				await this.loadConfig();
				this.render();
				this.attachListeners();
				this.showSuccess('Channel deleted successfully');
			} else {
				this.showError('Failed to delete channel');
			}
		} catch (error) {
			this.showError('Failed to delete channel: ' + error.message);
		}
	}

	async handleTestChannel(id) {
		try {
			const response = await fetch('/api/notifications/test', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					channel_id: id
				})
			});

			if (response.ok) {
				this.showSuccess('Test notification sent');
			} else {
				const error = await response.json();
				this.showError(error.error || 'Failed to send test notification');
			}
		} catch (error) {
			this.showError('Failed to send test notification: ' + error.message);
		}
	}

	async handleTestNotification() {
		try {
			const response = await fetch('/api/notifications/test', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({})
			});

			if (response.ok) {
				this.showSuccess('Test notification sent to all channels');
			} else {
				const error = await response.json();
				this.showError(error.error || 'Failed to send test notification');
			}
		} catch (error) {
			this.showError('Failed to send test notification: ' + error.message);
		}
	}

	async saveQuietHours() {
		const enabled = document.getElementById('quiet-hours-enabled').checked;
		const start = document.getElementById('quiet-hours-start').value.split(':');
		const end = document.getElementById('quiet-hours-end').value.split(':');

		// Collect day mask
		let daysMask = 0;
		document.querySelectorAll('.day-checkbox-input:checked').forEach(cb => {
			daysMask |= (1 << parseInt(cb.dataset.day));
		});

		const morningDigest = document.getElementById('morning-digest-enabled').checked;
		const digestTime = document.getElementById('digest-time').value.split(':');

		const quietHours = {
			enabled: enabled,
			start_hour: parseInt(start[0]),
			start_min: parseInt(start[1]),
			end_hour: parseInt(end[0]),
			end_min: parseInt(end[1]),
			days_mask: daysMask,
			morning_digest: morningDigest,
			digest_hour: parseInt(digestTime[0]),
			digest_min: parseInt(digestTime[1])
		};

		try {
			const response = await fetch('/api/notifications/quiet-hours', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(quietHours)
			});

			if (response.ok) {
				this.showSuccess('Quiet hours saved');
			} else {
				this.showError('Failed to save quiet hours');
			}
		} catch (error) {
			this.showError('Failed to save quiet hours: ' + error.message);
		}
	}

	async saveBatchingConfig() {
		const batching = {
			enabled: document.getElementById('batching-enabled').checked,
			batch_window_sec: parseInt(document.getElementById('batch-window').value),
			max_batch_size: parseInt(document.getElementById('max-batch-size').value),
			batch_low: document.getElementById('batch-low').checked,
			batch_medium: document.getElementById('batch-medium').checked
		};

		try {
			const response = await fetch('/api/notifications/batching', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(batching)
			});

			if (response.ok) {
				this.showSuccess('Batching settings saved');
			} else {
				this.showError('Failed to save batching settings');
			}
		} catch (error) {
			this.showError('Failed to save batching settings: ' + error.message);
		}
	}

	async saveEventTypes() {
		// Collect enabled event types
		const enabledTypes = {};
		document.querySelectorAll('.event-type-checkbox:checked').forEach(cb => {
			enabledTypes[cb.dataset.event] = true;
		});

		// Update all channels with the new event type settings
		for (const channel of this.channels) {
			try {
				await fetch(`/api/notifications/channels/${channel.id}`, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({
						...channel,
						enabled_types: enabledTypes
					})
				});
			} catch (error) {
				console.error('Failed to update channel event types:', error);
			}
		}

		this.showSuccess('Event types saved');
	}

	showSuccess(message) {
		// Show a toast notification
		this.showToast(message, 'success');
	}

	showError(message) {
		// Show a toast notification
		this.showToast(message, 'error');
	}

	showToast(message, type) {
		// Create toast element
		const toast = document.createElement('div');
		toast.className = `toast toast-${type}`;
		toast.textContent = message;
		document.body.appendChild(toast);

		// Auto-remove after 3 seconds
		setTimeout(() => {
			toast.classList.add('toast-hiding');
			setTimeout(() => toast.remove(), 300);
		}, 3000);
	}
}

// Initialize notification settings when settings panel is shown
document.addEventListener('settings-shown', (e) => {
	if (e.detail.panel === 'notifications') {
		const settings = new NotificationSettings(window.api);
		settings.init();
	}
});
