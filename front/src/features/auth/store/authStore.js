import { create } from 'zustand';
import { signInWithPopup, signOut, onAuthStateChanged } from 'firebase/auth';
import { auth, googleProvider } from '../../../shared/firebase/config';
import { loginApi } from '../api/login';

export const useAuthStore = create((set, get) => ({
  user: null,
  isAuthenticated: false,
  loading: true,
  error: null,

  // Initialize Firebase auth listener + ensure server token exists
  initAuth: () => {
    onAuthStateChanged(auth, async (firebaseUser) => {
      if (firebaseUser) {
        set({
          user: {
            uid: firebaseUser.uid,
            email: firebaseUser.email,
            displayName: firebaseUser.displayName,
            photoURL: firebaseUser.photoURL,
          },
          isAuthenticated: true,
          loading: false,
        });

        // If Firebase is logged in but server token is missing, exchange automatically.
        const serverToken = sessionStorage.getItem('access_token');
        if (!serverToken) {
          console.log('[auth] Firebase logged in but no server token — exchanging...');
          try {
            const idToken = await firebaseUser.getIdToken();
            const serverData = await loginApi({
              googleIdToken: idToken,
              firebaseToken: firebaseUser.uid,
              email: firebaseUser.email,
              displayName: firebaseUser.displayName,
            });
            sessionStorage.setItem('access_token', serverData.access_token);
            sessionStorage.setItem('refresh_token', serverData.refresh_token);
            sessionStorage.setItem('token_expires_at', String(serverData.expires_at));
            console.log('[auth] Server token acquired automatically');

            // Handle proxy callback if browser was opened by proxy.
            handleProxyCallback(serverData);
          } catch (err) {
            console.error('[auth] Auto token exchange failed:', err);
          }
        }
      } else {
        set({ user: null, isAuthenticated: false, loading: false });
      }
    });
  },

  // Google Sign-In → exchange Firebase token for server token pair
  signInWithGoogle: async () => {
    set({ error: null });
    try {
      const result = await signInWithPopup(auth, googleProvider);
      const idToken = await result.user.getIdToken();
      console.log('[auth] Firebase login success, exchanging for server token...');

      // Exchange Firebase token for server's access + refresh token
      const serverData = await loginApi({
        googleIdToken: idToken,
        firebaseToken: result.user.uid,
        email: result.user.email,
        displayName: result.user.displayName,
      });
      console.log('[auth] Server token received:', {
        hasAccessToken: !!serverData.access_token,
        hasRefreshToken: !!serverData.refresh_token,
        expiresAt: serverData.expires_at,
      });

      // Store SERVER tokens (not Firebase token)
      sessionStorage.setItem('access_token', serverData.access_token);
      sessionStorage.setItem('refresh_token', serverData.refresh_token);
      sessionStorage.setItem('token_expires_at', String(serverData.expires_at));

      // Handle proxy callback if present (proxy opened this browser window).
      // This will redirect, so it must happen BEFORE returning.
      if (handleProxyCallback(serverData)) {
        return true; // browser is redirecting to proxy callback
      }

      return true;
    } catch (err) {
      console.error('[auth] Login failed:', err);
      set({ error: err.message || String(err) });
      return false;
    }
  },

  // Sign Out
  logout: async () => {
    try {
      await signOut(auth);
      sessionStorage.removeItem('access_token');
      sessionStorage.removeItem('refresh_token');
      sessionStorage.removeItem('token_expires_at');
    } catch (err) {
      console.error('Logout failed:', err);
    }
  },
}));

// Save proxy_callback from URL to sessionStorage so it survives redirects.
// Called early — before AuthGuard can redirect away from /login.
(function captureProxyCallback() {
  const params = new URLSearchParams(window.location.search);
  const cb = params.get('proxy_callback');
  if (cb) {
    sessionStorage.setItem('proxy_callback', cb);
    console.log('[proxy-callback] Captured from URL:', cb);
  }
})();

// If the login page was opened by the proxy with ?proxy_callback=...,
// redirect to the callback URL with the server tokens.
// Checks both URL param and sessionStorage (survives redirect).
// Returns true if redirect will happen.
function handleProxyCallback(serverData) {
  const params = new URLSearchParams(window.location.search);
  const proxyCallback = params.get('proxy_callback') || sessionStorage.getItem('proxy_callback');

  if (!proxyCallback || !serverData.access_token) return false;

  // Clean up — one-time use.
  sessionStorage.removeItem('proxy_callback');

  try {
    // POST form submission — tokens go in the body, not the URL.
    // Avoids token exposure in browser history, Referer headers, and server logs.
    const form = document.createElement('form');
    form.method = 'POST';
    form.action = proxyCallback;
    form.style.display = 'none';

    for (const [name, value] of [
      ['access_token', serverData.access_token],
      ['refresh_token', serverData.refresh_token],
      ['expires_at', String(serverData.expires_at)],
    ]) {
      const input = document.createElement('input');
      input.type = 'hidden';
      input.name = name;
      input.value = value;
      form.appendChild(input);
    }

    document.body.appendChild(form);
    console.log('[proxy-callback] Posting tokens to proxy callback (secure form POST)');
    form.submit();
    return true;
  } catch (err) {
    console.error('[proxy-callback] Invalid callback URL:', err);
    return false;
  }
}
