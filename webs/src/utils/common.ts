import { getAppEnvConfig } from '/@/utils/env';

export const getWebsocketApiUrl = (path: Nullable<string>): string => {
  const host = window.location.host;
  const env = getAppEnvConfig();
  let preApi = '';
  if (env.VITE_GLOB_API_URL_PREFIX) {
    preApi = '/' + env.VITE_GLOB_API_URL_PREFIX;
  }
  return `ws://${host}${preApi}${path}`;
};
