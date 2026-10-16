import axios from 'axios';
import { getToken } from '../stores/useAuthStore';

const api = axios.create({ baseURL: import.meta.env.VITE_API_URL || 'http://127.0.0.1:5000/api/v1', timeout: 8000, headers: { 'Content-Type': 'application/json' } });
api.interceptors.request.use((config) => { const token = getToken(); if (token) config.headers.Authorization = `Bearer ${token}`; return config; });
export default api;
