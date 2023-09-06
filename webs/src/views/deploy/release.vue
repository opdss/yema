<template>
  <PageWrapper contentFullHeight title="发布上线">
    <a-card title="" v-loading="loadingRef">
      <alert v-if="alertMsg.msg" :type="alertMsg.type">
        <template #description>
          {{ alertMsg.msg }}
          <br />
          <a-button type="info" v-if="alertMsg.state == DeployStatus.Audit" @click="startRelease">
            开始上线
          </a-button>
        </template>
      </alert>

      <a-tabs :style="{ minHeight: '400px' }" v-model:activeKey="activeKey">
        <a-tab-pane
          class="text-white bg-gray-700"
          v-for="ss in servers"
          :key="ss.id"
          :tab="ss.host"
        >
          <pre class="mx-1">{{ ss.output }}</pre>
        </a-tab-pane>
      </a-tabs>
    </a-card>
  </PageWrapper>
</template>
<script lang="ts" setup>
  import { onMounted, reactive, ref, watchEffect } from 'vue';
  import { PageWrapper } from '/@/components/Page';
  import { Alert, Card as ACard, TabPane as ATabPane, Tabs as ATabs } from 'ant-design-vue';
  import { ListItem as DeployListItem, ReleaseOutput } from '/@/api/deploy/model';
  import { useRoute } from 'vue-router';
  import { DeployStatus, DeployStatusShowMsg } from '/@/enums/fieldEnum';
  import { useWebSocket } from '@vueuse/core';
  import { getDeployConsoleWs, detailDeploy, startDeploy } from '/@/api/deploy';
  import { useUserStore } from '/@/store/modules/user';

  type server = {
    id: number;
    host: string;
    output: string;
  };
  type serverItems = { [key: number]: server };
  type stateMsg = { type: 'success' | 'info' | 'error' | 'warning'; msg: string; state: number };

  const userStore = useUserStore();
  const route = useRoute();
  const loadingRef = ref(false);
  const activeKey = ref<number>(0);
  const errMsg = ref<string>('');
  const deployDetail = ref<DeployListItem | null>(null);
  const servers = ref<serverItems>({
    0: { id: 0, host: '127.0.0.1', output: '' },
  });
  const deployId = parseInt(route.params?.id as unknown as string);
  const alertMsg = reactive<stateMsg>({ type: 'error', msg: '', state: 0 });

  const { status, open: wsOpen } = useWebSocket(getDeployConsoleWs(deployId), {
    autoReconnect: false,
    heartbeat: false,
    immediate: false,
    protocols: [userStore.getToken, userStore.getCurrentSpaceId.toString()],
    onError: (_, event) => {
      alertMsg.type = 'error';
      alertMsg.msg = event.toString();
    },
    onDisconnected: () => {
      console.log('断开ws连接');
    },
    onMessage: (_, event) => {
      console.log(event.data);
      let data: ReleaseOutput;
      try {
        data = JSON.parse(event.data);
      } catch {
        return;
      }
      servers.value[data.server_id].output += data.data;
    },
  });

  async function init(id: number) {
    loadingRef.value = true;
    await detailDeploy(id, true)
      .then((res) => {
        deployDetail.value = res;
      })
      .catch((e) => {
        alertMsg.msg = (e as unknown as Error).toString();
      })
      .finally(() => {
        loadingRef.value = false;
      });

    if (deployDetail.value?.servers.length == 0) {
      alertMsg.type = 'error';
      alertMsg.msg = '该上线单发布服务器为空，请检查在执行操作！';
      return;
    }
    deployDetail.value?.servers.forEach((v) => {
      servers.value[v.id] = {
        id: v.id,
        host: v.host,
        output: '',
      };
    });
    console.log('servers', servers);
    let sm = DeployStatusShowMsg(deployDetail.value?.status as number);
    alertMsg.msg = sm[0];
    alertMsg.type = sm[1];
    alertMsg.state = deployDetail.value?.status;
    //打开websocket
    if (
      deployDetail.value?.status != DeployStatus.Waiting &&
      deployDetail.value?.status != DeployStatus.Audit
    ) {
      wsOpen();
    }
    //wsOpen()
  }

  function startRelease() {
    loadingRef.value = true;
    startDeploy(deployId, true)
      .then(() => {
        loadingRef.value = false;
        if (status.value != 'OPEN') {
          wsOpen();
        }
      })
      .catch((e) => {
        alertMsg.msg = e.toString();
        loadingRef.value = false;
      });
  }

  watchEffect(() => {
    if (status.value == 'CONNECTING') {
      loadingRef.value = true;
    } else {
      loadingRef.value = false;
    }
  });

  onMounted(() => {
    init(deployId);
  });
</script>
