import http from '../../../shared/api/http';
import { endpoints } from '../../../shared/api/endpoints';

export async function loginApi({ googleIdToken, firebaseToken, email, displayName }) {
  const { data } = await http.post(endpoints.auth.login, {
    google_id_token: googleIdToken,
    firebase_token: firebaseToken,
    email,
    display_name: displayName,
  });
  return data;
}

export async function signupApi({ googleIdToken, firebaseToken, email, displayName, promptText }) {
  const { data } = await http.post(endpoints.auth.signup, {
    google_id_token: googleIdToken,
    firebase_token: firebaseToken,
    email,
    display_name: displayName,
    prompt_text: promptText,
  });
  return data;
}
