class PerfTestDashboard {
    constructor() {
        this.ws = null;
        this.charts = {};
        this.currentSession = null;
        this.maxDataPoints = 300;

        this.initializeCharts();
        this.connectWebSocket();
        this.startDurationTimer();
    }

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ptest/ws`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            this.log('Connected to server');
            this.updateConnectionStatus(true);
        };

        this.ws.onclose = () => {
            this.log('Disconnected from server');
            this.updateConnectionStatus(false);
            // Attempt to reconnect after 3 seconds
            setTimeout(() => this.connectWebSocket(), 3000);
        };

        this.ws.onerror = (error) => {
            this.log(`WebSocket error: ${error}`);
        };

        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                this.handleMessage(message);
            } catch (error) {
                this.log(`Error parsing message: ${error}`);
            }
        };
    }

    handleMessage(message) {
        switch (message.type) {
            case 'session_start':
                this.handleSessionStart(message.data);
                break;
            case 'session_stop':
                this.handleSessionStop(message.data);
                break;
            case 'optimized_data':
                this.handleOptimizedData(message.data);
                break;
            case 'reset':
                this.resetCharts();
                break;
            default:
                this.log(`Unknown message type: ${message.type}`);
        }
    }

    handleSessionStart(sessionData) {
        this.currentSession = sessionData;
        this.updateSessionInfo(sessionData);
        this.resetCharts();
        this.log(`Started session: ${sessionData.session_name}`);
    }

    handleSessionStop(sessionData) {
        this.updateSessionInfo(sessionData);
        this.log(`Stopped session: ${sessionData.session_name}`);
    }

    handleOptimizedData(messageData) {
        if (!messageData) return;

        // Handle both old format (direct chart data) and new format (with session stats)
        let chartData, sessionStats;

        if (messageData.chart_data) {
            // New format with session stats
            chartData = messageData.chart_data;
            sessionStats = messageData.session_stats;
        } else {
            // Old format - direct chart data
            chartData = messageData;
        }

        // Use recent data for real-time updates
        const data = this.selectBestDataset(chartData);

        if (data && data.length > 0) {
            this.updateCharts(data);

            // Use session stats for total requests if available, otherwise fall back to latest stat
            if (sessionStats) {
                this.updateRealTimeStatsWithSession(data[data.length - 1], sessionStats);
            } else {
                this.updateRealTimeStats(data[data.length - 1]);
            }
        }
    }

    updateRealTimeStatsWithSession(latestStat, sessionStats) {
        if (!latestStat) return;

        // Use session stats for cumulative data
        const totalRequests = sessionStats.total_requests || 0;
        const currentTPS = (latestStat.TpsSuccess || 0) + (latestStat.TpsFailure || 0);

        // Use cumulative average instead of latest stat
        const avgResponseTime = sessionStats.cumulative_avg_rt || 0;

        document.getElementById('totalRequests').textContent = totalRequests.toLocaleString();
        document.getElementById('currentTPS').textContent = Math.round(currentTPS);
        document.getElementById('avgResponseTime').textContent = Math.round(avgResponseTime);
        document.getElementById('errorRate').textContent = `${(latestStat.ErrorRate || 0).toFixed(1)}%`;

        // Log for debugging
        console.log(`Success RT: ${Math.round(latestStat.ResponseTime || 0)}ms, Error RT: ${Math.round(latestStat.FailureResponseTime || 0)}ms, Overall Avg: ${Math.round(avgResponseTime)}ms`);
    }

    updateRealTimeStats(latestStat) {
        if (!latestStat) return;

        // Fallback for old format - this still shows per-second data
        const totalRequests = (latestStat.SuccessCount || 0) + (latestStat.FailureCount || 0);
        const currentTPS = (latestStat.TpsSuccess || 0) + (latestStat.TpsFailure || 0);

        document.getElementById('totalRequests').textContent = totalRequests.toLocaleString();
        document.getElementById('currentTPS').textContent = Math.round(currentTPS);
        document.getElementById('avgResponseTime').textContent = Math.round(latestStat.ResponseTime || 0);
        document.getElementById('errorRate').textContent = `${(latestStat.ErrorRate || 0).toFixed(1)}%`;
    }

    selectBestDataset(chartData) {
        // Priority: recent -> medium -> longterm
        if (chartData.recent && chartData.recent.length > 0) {
            return chartData.recent;
        } else if (chartData.medium && chartData.medium.length > 0) {
            return chartData.medium;
        } else if (chartData.longterm && chartData.longterm.length > 0) {
            return chartData.longterm;
        }
        return [];
    }

    updateCharts(data) {
        const labels = [];
        const totalTPSData = [];
        const successTPSData = [];
        const errorTPSData = [];
        const successResponseTimeData = [];
        const successResponseTime90Data = [];
        const successResponseTime95Data = [];
        const successResponseTime99Data = [];
        const errorResponseTimeData = [];
        const errorResponseTime90Data = [];
        const errorResponseTime95Data = [];
        const errorResponseTime99Data = [];
        const errorRateData = [];

        const startTime = data[0]?.Time || 0;

        data.forEach((stat, index) => {
            const relativeTime = stat.Time - startTime;
            labels.push(relativeTime);

            // TPS data
            const totalTPS = (stat.TpsSuccess || 0) + (stat.TpsFailure || 0);
            totalTPSData.push(totalTPS);
            successTPSData.push(stat.TpsSuccess || 0);
            errorTPSData.push(stat.TpsFailure || 0);

            // Success Response Time data
            successResponseTimeData.push(stat.ResponseTime || 0);
            successResponseTime90Data.push(stat.ResponseTime90 || 0);
            successResponseTime95Data.push(stat.ResponseTime95 || 0);
            successResponseTime99Data.push(stat.ResponseTime99 || 0);

            // Error Response Time data
            errorResponseTimeData.push(stat.FailureResponseTime || 0);
            errorResponseTime90Data.push(stat.FailureResponseTime90 || 0);
            errorResponseTime95Data.push(stat.FailureResponseTime95 || 0);
            errorResponseTime99Data.push(stat.FailureResponseTime99 || 0);

            // Error Rate data
            errorRateData.push(stat.ErrorRate || 0);
        });

        // Update Total TPS chart
        this.updateChartData(this.charts.totalTPS, labels, [{
            label: 'Total TPS (Success + Error)',
            data: totalTPSData,
            borderColor: 'rgb(123, 104, 238)',
            backgroundColor: 'rgba(123, 104, 238, 0.1)',
            fill: true
        }]);

        // Update Success TPS chart
        this.updateChartData(this.charts.successTPS, labels, [{
            label: 'Success TPS',
            data: successTPSData,
            borderColor: 'rgb(75, 192, 192)',
            backgroundColor: 'rgba(75, 192, 192, 0.1)',
            fill: true
        }]);

        // Update Error TPS chart
        this.updateChartData(this.charts.errorTPS, labels, [{
            label: 'Error TPS',
            data: errorTPSData,
            borderColor: 'rgb(255, 99, 132)',
            backgroundColor: 'rgba(255, 99, 132, 0.1)',
            fill: true
        }]);

        // Update Success Response Time chart
        this.updateChartData(this.charts.successResponseTime, labels, [
            {
                label: 'Average',
                data: successResponseTimeData,
                borderColor: 'rgb(54, 162, 235)',
                backgroundColor: 'rgba(54, 162, 235, 0.1)',
                fill: false
            },
            {
                label: '90th Percentile',
                data: successResponseTime90Data,
                borderColor: 'rgb(255, 206, 86)',
                backgroundColor: 'rgba(255, 206, 86, 0.1)',
                fill: false
            },
            {
                label: '95th Percentile',
                data: successResponseTime95Data,
                borderColor: 'rgb(255, 159, 64)',
                backgroundColor: 'rgba(255, 159, 64, 0.1)',
                fill: false
            },
            {
                label: '99th Percentile',
                data: successResponseTime99Data,
                borderColor: 'rgb(255, 99, 132)',
                backgroundColor: 'rgba(255, 99, 132, 0.1)',
                fill: false
            }
        ]);

        // Update Error Response Time chart
        this.updateChartData(this.charts.errorResponseTime, labels, [
            {
                label: 'Average',
                data: errorResponseTimeData,
                borderColor: 'rgb(220, 53, 69)',
                backgroundColor: 'rgba(220, 53, 69, 0.1)',
                fill: false
            },
            {
                label: '90th Percentile',
                data: errorResponseTime90Data,
                borderColor: 'rgb(255, 193, 7)',
                backgroundColor: 'rgba(255, 193, 7, 0.1)',
                fill: false
            },
            {
                label: '95th Percentile',
                data: errorResponseTime95Data,
                borderColor: 'rgb(253, 126, 20)',
                backgroundColor: 'rgba(253, 126, 20, 0.1)',
                fill: false
            },
            {
                label: '99th Percentile',
                data: errorResponseTime99Data,
                borderColor: 'rgb(214, 51, 132)',
                backgroundColor: 'rgba(214, 51, 132, 0.1)',
                fill: false
            }
        ]);

        // Update Error Rate chart
        this.updateChartData(this.charts.errorRate, labels, [{
            label: 'Error Rate (%)',
            data: errorRateData,
            borderColor: 'rgb(255, 99, 132)',
            backgroundColor: 'rgba(255, 99, 132, 0.1)',
            fill: true
        }]);
    }

    updateChartData(chart, labels, datasets) {
        chart.data.labels = labels;
        chart.data.datasets = datasets;
        chart.update('none'); // Disable animation for better performance
    }

    updateSessionInfo(sessionData) {
        document.getElementById('sessionName').textContent = sessionData.session_name || 'No active session';

        const statusElement = document.getElementById('sessionStatus');
        statusElement.textContent = sessionData.status;
        statusElement.className = `status ${sessionData.status}`;

        this.currentSession = sessionData;
    }

    updateConnectionStatus(connected) {
        const statusElement = document.getElementById('connectionStatus');
        statusElement.textContent = connected ? 'Connected' : 'Disconnected';
        statusElement.className = `connection-status ${connected ? 'connected' : 'disconnected'}`;
    }

    initializeCharts() {
        const chartConfig = {
            type: 'line',
            options: {
                responsive: true,
                interaction: {
                    intersect: false,
                    mode: 'index'
                },
                scales: {
                    x: {
                        display: true,
                        title: {
                            display: true,
                            text: 'Time (seconds)'
                        }
                    },
                    y: {
                        display: true,
                        beginAtZero: true
                    }
                },
                elements: {
                    point: {
                        radius: 0 // Hide points for better performance
                    }
                },
                animation: false, // Disable animations for better performance
                plugins: {
                    legend: {
                        display: true,
                        position: 'top'
                    }
                }
            }
        };

        // Initialize all 6 charts
        this.charts.totalTPS = new Chart(
            document.getElementById('totalTPSChart').getContext('2d'),
            { ...chartConfig, data: { labels: [], datasets: [] } }
        );

        this.charts.successTPS = new Chart(
            document.getElementById('successTPSChart').getContext('2d'),
            { ...chartConfig, data: { labels: [], datasets: [] } }
        );

        this.charts.errorTPS = new Chart(
            document.getElementById('errorTPSChart').getContext('2d'),
            { ...chartConfig, data: { labels: [], datasets: [] } }
        );

        this.charts.successResponseTime = new Chart(
            document.getElementById('successResponseTimeChart').getContext('2d'),
            { ...chartConfig, data: { labels: [], datasets: [] } }
        );

        this.charts.errorResponseTime = new Chart(
            document.getElementById('errorResponseTimeChart').getContext('2d'),
            { ...chartConfig, data: { labels: [], datasets: [] } }
        );

        this.charts.errorRate = new Chart(
            document.getElementById('errorRateChart').getContext('2d'),
            { ...chartConfig, data: { labels: [], datasets: [] } }
        );
    }

    resetCharts() {
        Object.values(this.charts).forEach(chart => {
            chart.data.labels = [];
            chart.data.datasets = [];
            chart.update();
        });

        // Reset stats display
        document.getElementById('totalRequests').textContent = '0';
        document.getElementById('currentTPS').textContent = '0';
        document.getElementById('avgResponseTime').textContent = '0';
        document.getElementById('errorRate').textContent = '0%';
    }

    startDurationTimer() {
        setInterval(() => {
            if (this.currentSession && this.currentSession.status === 'running') {
                const startTime = new Date(this.currentSession.start_time);
                const duration = Math.floor((Date.now() - startTime.getTime()) / 1000);
                const minutes = Math.floor(duration / 60);
                const seconds = duration % 60;
                document.getElementById('sessionDuration').textContent =
                    `Duration: ${minutes}m ${seconds}s`;
            } else {
                document.getElementById('sessionDuration').textContent = '-';
            }
        }, 1000);
    }

    log(message) {
        const logElement = document.getElementById('log');
        const timestamp = new Date().toLocaleTimeString();
        const logEntry = document.createElement('div');
        logEntry.textContent = `[${timestamp}] ${message}`;

        logElement.appendChild(logEntry);
        logElement.scrollTop = logElement.scrollHeight;

        // Keep only last 100 log entries
        while (logElement.children.length > 100) {
            logElement.removeChild(logElement.firstChild);
        }
    }
}

// Initialize dashboard when page loads
window.addEventListener('load', () => {
    new PerfTestDashboard();
});