import AsyncStorage from '@react-native-async-storage/async-storage';
import { NavigationContainer } from '@react-navigation/native';
import { createNativeStackNavigator, type NativeStackScreenProps } from '@react-navigation/native-stack';
import { StatusBar } from 'expo-status-bar';
import type { ReactNode } from 'react';
import { useEffect, useMemo, useState } from 'react';
import { SafeAreaView } from 'react-native-safe-area-context';
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  Text,
  TextInput,
  View,
} from 'react-native';

type RequestState = 'idle' | 'checking' | 'sending' | 'ok' | 'error';
type PasteChord = 'ctrl_v' | 'ctrl_shift_v';
type RootStackParamList = {
  Settings: undefined;
  Home: undefined;
};

const Stack = createNativeStackNavigator<RootStackParamList>();

const defaultUrl = 'http://192.168.1.100:47832';
const recentUrlsKey = 'voice_push_recent_urls';
const maxRecentUrls = 5;

export default function App() {
  const [daemonUrl, setDaemonUrl] = useState(defaultUrl);
  const [token, setToken] = useState('');
  const [text, setText] = useState('');
  const [pasteChord, setPasteChord] = useState<PasteChord>('ctrl_v');
  const [state, setState] = useState<RequestState>('idle');
  const [message, setMessage] = useState('Ready');
  const [recentUrls, setRecentUrls] = useState<string[]>([]);

  const normalizedUrl = useMemo(() => normalizeBaseUrl(daemonUrl), [daemonUrl]);
  const busy = state === 'checking' || state === 'sending';
  const hasConfig = normalizedUrl.length > 0 && token.trim().length > 0;
  const canSend = hasConfig && text.trim().length > 0 && !busy;

  useEffect(() => {
    let active = true;

    AsyncStorage.getItem(recentUrlsKey)
      .then((value) => {
        if (!active || !value) {
          return;
        }

        const parsed = JSON.parse(value);
        if (Array.isArray(parsed)) {
          setRecentUrls(parsed.filter((item): item is string => typeof item === 'string'));
        }
      })
      .catch(() => {
        if (active) {
          setRecentUrls([]);
        }
      });

    return () => {
      active = false;
    };
  }, []);

  async function rememberCurrentUrl() {
    if (!normalizedUrl) {
      return;
    }

    const next = [normalizedUrl, ...recentUrls.filter((url) => url !== normalizedUrl)].slice(0, maxRecentUrls);
    setRecentUrls(next);
    await AsyncStorage.setItem(recentUrlsKey, JSON.stringify(next));
  }

  async function checkHealth() {
    if (!normalizedUrl) {
      setState('error');
      setMessage('Missing daemon URL');
      return;
    }

    setState('checking');
    setMessage('Checking...');

    try {
      const response = await fetch(`${normalizedUrl}/health`);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      setState('ok');
      setMessage('Daemon online');
      await rememberCurrentUrl();
    } catch (error) {
      setState('error');
      setMessage(errorMessage(error));
    }
  }

  async function sendText() {
    if (!canSend) {
      setState('error');
      setMessage('Missing URL, token, or text');
      return;
    }

    setState('sending');
    setMessage('Sending...');

    try {
      const response = await fetch(`${normalizedUrl}/type`, {
        method: 'POST',
        headers: {
          Authorization: `Bearer ${token.trim()}`,
          'Content-Type': 'text/plain; charset=utf-8',
          'X-Voice-Paste-Chord': pasteChord,
        },
        body: text,
      });

      if (response.status === 204) {
        setState('ok');
        setMessage('Sent');
        setText('');
        await rememberCurrentUrl();
        return;
      }

      const body = await response.text();
      throw new Error(body.trim() || `HTTP ${response.status}`);
    } catch (error) {
      setState('error');
      setMessage(errorMessage(error));
    }
  }

  function resetStatus() {
    setState('idle');
    setMessage('Ready');
  }

  return (
    <NavigationContainer>
      <StatusBar style="dark" />
      <Stack.Navigator
        initialRouteName="Settings"
        screenOptions={{
          contentStyle: { backgroundColor: '#fffbfe' },
          headerBackTitle: 'Back',
          headerShadowVisible: false,
          headerStyle: { backgroundColor: '#fffbfe' },
          headerTintColor: '#006a6a',
          headerTitleStyle: { color: '#1d1b20', fontWeight: '700' },
        }}
      >
        <Stack.Screen name="Settings" options={{ title: 'Settings' }}>
          {(props) => (
            <SettingsScreen
              {...props}
              busy={busy}
              checkHealth={checkHealth}
              daemonUrl={daemonUrl}
              hasConfig={hasConfig}
              message={message}
              recentUrls={recentUrls}
              rememberCurrentUrl={rememberCurrentUrl}
              resetStatus={resetStatus}
              setDaemonUrl={setDaemonUrl}
              setToken={setToken}
              state={state}
              token={token}
            />
          )}
        </Stack.Screen>
        <Stack.Screen
          name="Home"
          options={{
            headerRight: () => (
              <Pressable
                android_ripple={{ color: '#e6e0e9', foreground: true }}
                className={`min-h-9 justify-center overflow-hidden rounded-full px-3 ${
                  busy ? 'opacity-50' : ''
                }`}
                disabled={busy}
                onPress={checkHealth}
              >
                <Text className="text-sm font-bold text-[#006a6a]">Health</Text>
              </Pressable>
            ),
            title: 'Voice Push',
          }}
        >
          {(props) => (
            <HomeScreen
              {...props}
              canSend={canSend}
              message={message}
              pasteChord={pasteChord}
              sendText={sendText}
              setPasteChord={setPasteChord}
              setText={setText}
              state={state}
              text={text}
            />
          )}
        </Stack.Screen>
      </Stack.Navigator>
    </NavigationContainer>
  );
}

type SettingsProps = NativeStackScreenProps<RootStackParamList, 'Settings'> & {
  busy: boolean;
  checkHealth: () => Promise<void>;
  daemonUrl: string;
  hasConfig: boolean;
  message: string;
  recentUrls: string[];
  rememberCurrentUrl: () => Promise<void>;
  resetStatus: () => void;
  setDaemonUrl: (value: string) => void;
  setToken: (value: string) => void;
  state: RequestState;
  token: string;
};

function SettingsScreen({
  busy,
  checkHealth,
  daemonUrl,
  hasConfig,
  message,
  navigation,
  recentUrls,
  rememberCurrentUrl,
  resetStatus,
  setDaemonUrl,
  setToken,
  state,
  token,
}: SettingsProps) {
  async function continueToHome() {
    await rememberCurrentUrl();
    resetStatus();
    navigation.push('Home');
  }

  return (
    <ScreenShell>
      <StatusMessage message={message} state={state} />

      <View className="gap-4">
        <Field label="Daemon URL">
          <TextInput
            autoCapitalize="none"
            autoCorrect={false}
            className="min-h-14 rounded border border-[#79747e] bg-[#fffbfe] px-4 py-3 text-base text-[#1d1b20]"
            inputMode="url"
            onChangeText={setDaemonUrl}
            placeholder="http://192.168.1.100:47832"
            placeholderTextColor="#79747e"
            value={daemonUrl}
          />
        </Field>

        {recentUrls.length > 0 ? (
          <View className="gap-2">
            <Text className="text-sm font-bold text-slate-700">Recent</Text>
            <View className="gap-2">
              {recentUrls.map((url) => (
                <Pressable
                  android_ripple={{ color: '#e6e0e9' }}
                  className="overflow-hidden rounded-xl bg-[#f3edf7] px-4 py-3"
                  key={url}
                  onPress={() => setDaemonUrl(url)}
                >
                  <Text className="text-sm font-semibold text-[#1d1b20]">{url}</Text>
                </Pressable>
              ))}
            </View>
          </View>
        ) : null}

        <Field label="Token">
          <TextInput
            autoCapitalize="none"
            autoCorrect={false}
            className="min-h-14 rounded border border-[#79747e] bg-[#fffbfe] px-4 py-3 text-base text-[#1d1b20]"
            onChangeText={setToken}
            placeholder="VOICE_DAEMON_TOKEN"
            placeholderTextColor="#79747e"
            secureTextEntry
            value={token}
          />
        </Field>

        <View className="flex-row gap-3">
          <Pressable
            android_ripple={{ color: '#e6e0e9', foreground: true }}
            className={`min-h-12 flex-1 items-center justify-center overflow-hidden rounded-full border border-[#79747e] bg-[#fffbfe] px-5 ${
              busy ? 'opacity-50' : ''
            }`}
            disabled={busy}
            onPress={checkHealth}
          >
            {state === 'checking' ? (
              <ActivityIndicator color="#006a6a" />
            ) : (
              <Text className="text-sm font-bold text-[#006a6a]">Health</Text>
            )}
          </Pressable>

          <Pressable
            android_ripple={{ color: '#4f8f8e', foreground: true }}
            className={`min-h-12 flex-1 items-center justify-center overflow-hidden rounded-full bg-[#006a6a] px-5 ${
              !hasConfig ? 'opacity-50' : ''
            }`}
            disabled={!hasConfig}
            onPress={continueToHome}
          >
            <Text className="text-sm font-bold text-white">Continue</Text>
          </Pressable>
        </View>
      </View>
    </ScreenShell>
  );
}

type HomeProps = NativeStackScreenProps<RootStackParamList, 'Home'> & {
  canSend: boolean;
  message: string;
  pasteChord: PasteChord;
  sendText: () => Promise<void>;
  setPasteChord: (value: PasteChord) => void;
  setText: (value: string) => void;
  state: RequestState;
  text: string;
};

function HomeScreen({
  canSend,
  message,
  pasteChord,
  sendText,
  setPasteChord,
  setText,
  state,
  text,
}: HomeProps) {
  return (
    <ScreenShell>
      <StatusMessage message={message} state={state} />

      <View className="gap-3">
        <Field label="Text">
          <TextInput
            className="min-h-52 rounded border border-[#79747e] bg-[#fffbfe] px-4 py-3 text-base text-[#1d1b20]"
            multiline
            onChangeText={setText}
            placeholder="要发送到当前焦点窗口的文本"
            placeholderTextColor="#79747e"
            textAlignVertical="top"
            value={text}
          />
        </Field>

        <View className="flex-row items-center justify-between gap-3">
          <Text className="text-xs font-bold uppercase text-[#49454f]">Paste Mode</Text>
          <View className="w-48 flex-row rounded-full border border-[#79747e] bg-[#fffbfe] p-0.5">
            <ModeButton
              active={pasteChord === 'ctrl_v'}
              label="GUI"
              onPress={() => setPasteChord('ctrl_v')}
            />
            <ModeButton
              active={pasteChord === 'ctrl_shift_v'}
              label="Terminal"
              onPress={() => setPasteChord('ctrl_shift_v')}
            />
          </View>
        </View>
      </View>

      <View className="gap-2">
        <Pressable
          android_ripple={{ color: '#4f8f8e', foreground: true }}
          className={`min-h-16 items-center justify-center overflow-hidden rounded-full bg-[#006a6a] px-6 ${
            !canSend ? 'opacity-50' : ''
          }`}
          disabled={!canSend}
          onPress={sendText}
        >
          {state === 'sending' ? (
            <ActivityIndicator color="#ffffff" />
          ) : (
            <Text className="text-lg font-bold text-white">Send</Text>
          )}
        </Pressable>
      </View>
    </ScreenShell>
  );
}

function ScreenShell({ children }: { children: ReactNode }) {
  return (
    <SafeAreaView className="flex-1 bg-[#fffbfe]" edges={['left', 'right', 'bottom']}>
      <KeyboardAvoidingView
        behavior={Platform.OS === 'ios' ? 'padding' : undefined}
        className="flex-1"
      >
        <ScrollView
          className="flex-1"
          contentContainerClassName="grow gap-4 px-5 py-4"
          keyboardShouldPersistTaps="handled"
        >
          {children}
        </ScrollView>
      </KeyboardAvoidingView>
    </SafeAreaView>
  );
}

function StatusMessage({
  message,
  state,
}: {
  message: string;
  state: RequestState;
}) {
  return (
    <Text className={`overflow-hidden rounded-xl px-4 py-3 text-[15px] font-semibold ${statusClass(state)}`}>
      {message}
    </Text>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <View className="gap-2">
      <Text className="text-sm font-bold text-[#49454f]">{label}</Text>
      {children}
    </View>
  );
}

function ModeButton({
  active,
  label,
  onPress,
}: {
  active: boolean;
  label: string;
  onPress: () => void;
}) {
  return (
    <Pressable
      android_ripple={{ color: active ? '#b0cccb' : '#e6e0e9', foreground: true }}
      className={`min-h-9 flex-1 items-center justify-center overflow-hidden rounded-full px-2 ${
        active ? 'bg-[#cce8e7]' : 'bg-[#fffbfe]'
      }`}
      onPress={onPress}
    >
      <Text className={`text-xs font-bold ${active ? 'text-[#002020]' : 'text-[#49454f]'}`}>
        {label}
      </Text>
    </Pressable>
  );
}

function normalizeBaseUrl(value: string) {
  return value.trim().replace(/\/+$/, '');
}

function errorMessage(error: unknown) {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return 'Request failed';
}

function statusClass(state: RequestState) {
  if (state === 'ok') {
    return 'bg-[#cce8e7] text-[#002020]';
  }
  if (state === 'error') {
    return 'bg-[#ffdad6] text-[#410002]';
  }
  return 'bg-[#f3edf7] text-[#1d1b20]';
}
