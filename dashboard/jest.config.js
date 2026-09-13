module.exports = {
    testEnvironment: 'jsdom',
    testMatch: ['**/*.test.js'],
    setupFiles: ['./js/onboard.test.setup.js'],
    setupFilesAfterEnv: ['./js/ambient.test.setup.js'],
    // Timing-sensitive suites (wall-clock perf assertions, real-timer RAF
    // loop in the ambient setup) flake when a worker per core contends with
    // this shared box's other load. Run suites sequentially for
    // deterministic scheduling; total wall time is still ~30s.
    maxWorkers: 1,
};
