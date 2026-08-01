module.exports = {
    testEnvironment: 'jsdom',
    testMatch: ['**/*.test.js'],
    setupFiles: ['./js/onboard.test.setup.js'],
    setupFilesAfterEnv: ['./js/ambient.test.setup.js'],
};
