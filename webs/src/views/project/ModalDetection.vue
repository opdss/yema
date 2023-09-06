<template>
  <BasicModal
    v-bind="$attrs"
    @register="register"
    :title="'项目检测-' + projectDetail?.name"
    width="800px"
    @visible-change="handleVisibleChange"
    :closable="false"
    :keyboard="false"
    :maskClosable="false"
  >
    <Result
      v-if="isSuccessRef"
      status="success"
      title="检测成功"
      sub-title="现在可以自由的对该项目进行发布上线操作了"
    />
    <div
      v-if="!isSuccessRef"
      class="pt-3px pr-3px"
      v-loading="loadingRef"
      loading-tip="'项目检测中...'"
    >
      <a-list item-layout="horizontal" :data-source="detectionList">
        <template #renderItem="{ item }">
          <a-list-item>
            <a-list-item-meta
              :description="item.todo + (item.error ? '  (错误提示:' + item.error + ')' : '')"
            >
              <template #title>
                {{ item.title }}
              </template>
              <template #avatar>
                <CloseCircleOutlined style="color: red; font-size: 18px" />
              </template>
            </a-list-item-meta>
          </a-list-item>
        </template>
      </a-list>
    </div>
  </BasicModal>
</template>
<script lang="ts" setup>
  import { BasicModal, useModalInner } from '/@/components/Modal';
  import { DetectionInfoItem } from '/@/api/project/model';
  import { CloseCircleOutlined } from '@ant-design/icons-vue';
  import { useUserStore } from '/@/store/modules/user';
  import { reactive, ref, nextTick, watchEffect } from 'vue';
  import {
    Result,
    List as AList,
    ListItem as AListItem,
    ListItemMeta as AListItemMeta,
  } from 'ant-design-vue';
  import { useWebSocket } from '@vueuse/core';
  import { getDetectionProjectWs } from '/@/api/project';

  const userStore = useUserStore();
  const loadingRef = ref(true);
  const isSuccessRef = ref(false);
  let detectionList = reactive<DetectionInfoItem[]>([]);

  const props = defineProps({
    projectDetail: { type: Object },
  });

  const [register] = useModalInner((data) => {
    console.log('useModalInner', data);
  });

  const {
    status,
    close: wsClose,
    open: wsOpen,
  } = useWebSocket(getDetectionProjectWs(props.projectDetail?.id as number), {
    autoReconnect: false,
    immediate: false,
    protocols: [userStore.getToken, userStore.getCurrentSpaceId.toString()],
    onConnected: () => {},
    onError: (ws, event) => {
      loadingRef.value = false;
      console.log('连接失败', ws, event);
    },
    onDisconnected: (ws, event) => {
      loadingRef.value = false;
      console.log('连接断开', ws, event);
    },
    onMessage: (ws, event) => {
      console.log(ws, event);
      if (event.data == 'success') {
        isSuccessRef.value = true;
        loadingRef.value = false;
      } else if (event.data == 'error') {
        loadingRef.value = false;
      } else {
        let obj: DetectionInfoItem = JSON.parse(event.data);
        if (obj.server_id) {
          let s = servers[obj.server_id];
          if (s) {
            obj.title = obj.title + '[' + s.user + '@' + s.host + ':' + s.port + ']';
          }
        }
        detectionList.push(obj);
      }
    },
  });

  watchEffect(() => {
    console.log('status.value', status.value);
  });

  let servers = {};
  function handleVisibleChange(v) {
    if (v && props.projectDetail) {
      nextTick(() => {
        wsOpen();
      });
    } else {
      console.log('test');
      wsClose();
      detectionList = reactive<DetectionInfoItem[]>([]);
      isSuccessRef.value = false;
      loadingRef.value = true;
    }
  }
</script>
