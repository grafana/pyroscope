import { defineConfig } from 'cypress';

export default defineConfig({
  e2e: {
    baseUrl: 'http://localhost:8080/foobar/',
    env: {
      apiBasePath: '/foobar',
    },
    video: false,
  },
});
