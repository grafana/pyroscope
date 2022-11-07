async function main() {
  // eslint-disable-next-line global-require
  const { OAuth2Server } = require('oauth2-mock-server');

  const server = new OAuth2Server(undefined, undefined, {
    endpoints: {
      wellKnownDocument: '/.well-known/openid-configuration',
      token: '/token',
      jwks: '/jwks',
      authorize: '/authorize',
      userinfo: '/user',
      revoke: '/revoke',
      endSession: '/endSession',
      introspect: '/introspect',
    },
  });

  // hack to add /groups support to the mock server
  server.service.requestHandler.get('/groups', (req, res) => {
    res.status(200).json([{ path: 'allowed-group-example' }]);
  });

  server.service.addListener('beforeUserinfo', (userInfoResponse, req) => {
    userInfoResponse.body = {
      id: 1245,
      email: 'test@test.com',
      username: 'testuser',
      avatarurl:
        'https://www.gravatar.com/avatar/205e460b479e2e5b48aec07710c08d50',
    };
  });

  // Generate a new RSA key and add it to the keystore
  await server.issuer.keys.generate('RS256');

  // Start the server
  await server.start(18080, '0.0.0.0');
  console.log('Issuer URL:', server.issuer.url); // -> http://localhost:8080

  // Do some work with the server
  // ...

  // Stop the server
  // return await server.stop();
}

main();
