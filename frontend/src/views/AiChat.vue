<template>
  <div class="aichat-page">
    <el-card shadow="never">
      <template #header>
        <div class="card-header">
          <span>AI Chat</span>
          <el-button size="small" @click="clearChat" :disabled="loading">
            Clear
          </el-button>
        </div>
      </template>

      <div class="chat-messages" ref="chatContainer">
        <div
          v-for="(msg, i) in messages"
          :key="i"
          :class="['message-row', msg.role]"
        >
          <div class="bubble">{{ msg.content }}</div>
        </div>
        <div v-if="loading" class="message-row ai">
          <div class="bubble thinking"><el-icon class="is-loading" style="margin-right: 6px;"><Loading /></el-icon>Thinking...</div>
        </div>
      </div>

      <el-empty
        v-if="!loading && messages.length === 0"
        description="No messages yet"
        style="margin-top: 24px;"
      />

      <div class="chat-input-bar">
        <el-input
          v-model="input"
          placeholder="Ask about your OCI instances..."
          :disabled="loading"
          clearable
          @keyup.enter="send"
        />
        <el-button type="primary" :loading="loading" @click="send">
          Send
        </el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { post } from '../api/index.js'

const messages = ref([
  { role: 'ai', content: 'Ask me about your OCI instances...' }
])
const input = ref('')
const loading = ref(false)
const chatContainer = ref(null)

function scrollToBottom() {
  nextTick(() => {
    if (chatContainer.value) {
      chatContainer.value.scrollTop = chatContainer.value.scrollHeight
    }
  })
}

async function send() {
  const msg = input.value
  if (!msg.trim()) return

  messages.value.push({ role: 'user', content: msg })
  input.value = ''
  loading.value = true
  scrollToBottom()

  try {
    const r = await post('/ai/chat', { messages: [{ role: 'user', content: msg }] })
    messages.value.push({ role: 'ai', content: r.reply || 'No response' })
  } catch (e) {
    const detail = e.response?.data?.error || 'AI service unavailable'
    messages.value.push({ role: 'ai', content: 'Error: ' + detail })
    ElMessage.error('Failed to get AI response')
  } finally {
    loading.value = false
    scrollToBottom()
  }
}

function clearChat() {
  messages.value = [
    { role: 'ai', content: 'Ask me about your OCI instances...' }
  ]
}

onMounted(() => {
  scrollToBottom()
})
</script>

<style scoped>
.aichat-page {
  padding: 0;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-header > span {
  font-size: 18px;
  font-weight: 600;
}

.chat-messages {
  max-height: 400px;
  overflow-y: auto;
  padding: 12px;
  background: var(--el-bg-color-dark, #1a1a2e);
  border: 1px solid var(--el-border-color-lighter);
  border-radius: 6px;
  margin-bottom: 12px;
}

.message-row {
  display: flex;
  margin-bottom: 12px;
}

.message-row.user {
  justify-content: flex-end;
}

.message-row.ai {
  justify-content: flex-start;
}

.bubble {
  max-width: 80%;
  padding: 10px 14px;
  border-radius: 12px;
  font-size: 14px;
  line-height: 1.5;
  word-break: break-word;
  white-space: pre-wrap;
}

.message-row.user .bubble {
  background: #409eff;
  color: #fff;
  border-bottom-right-radius: 4px;
}

.message-row.ai .bubble {
  background: var(--el-fill-color-light, #f0f0f0);
  color: var(--el-text-color-primary, #303133);
  border-bottom-left-radius: 4px;
}

.bubble.thinking {
  display: flex;
  align-items: center;
  color: var(--el-text-color-secondary, #909399);
  font-style: italic;
}

.chat-input-bar {
  display: flex;
  gap: 12px;
  align-items: center;
}
</style>
