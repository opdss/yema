<template>
  <BasicModal
    v-bind="$attrs"
    @register="register"
    :title="'项目检测-' + projectDetail?.name"
    width="700px"
    @visible-change="handleVisibleChange"
  >
    <div class="pt-3px pr-3px" v-loading="loadingRef" v-loading-tip="'项目检测中...'">
      <a-list item-layout="horizontal" :data-source="detectionList">
        <template #renderItem="{ item }">
          <a-list-item>
            <a-list-item-meta :description="item.todo">
              <template #title>
                {{ item.title }}
              </template>
              <template #avatar>
                <CloseCircleOutlined style="color: red; font-size: 18px" />
              </template>
            </a-list-item-meta>
            {{ item.error }}
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
  import { useMessage } from '/@/hooks/web/useMessage';
  import { reactive, ref, nextTick } from 'vue';
  import { useLoading } from '/@/components/Loading';
  import {
    List as AList,
    ListItem as AListItem,
    ListItemMeta as AListItemMeta,
  } from 'ant-design-vue';
  import { useWebSocket } from '@vueuse/core';
  import { getDetectionProjectWs } from '/@/api/project';

  const userStore = useUserStore();
  const loadingRef = ref(true);
  const { createMessage } = useMessage();
  const detectionList = reactive<DetectionInfoItem[]>([]);
  const wrapEl = ref<ElRef>(null);

  const props = defineProps({
    projectDetail: { type: Object },
  });

  const [register] = useModalInner((data) => {
    console.log('useModalInner', data);
  });

  let closeFn: Function | null = null;
  function handleVisibleChange(v) {
    console.log('handleVisibleChange', v, props.projectDetail);
    //v && props.userData && nextTick(() => onDataReceive(props.userData));
    if (v && props.projectDetail) {
      nextTick(() => {
        closeFn = openWs(props.projectDetail?.id as number);
      });
    } else {
      if (closeFn) {
        closeFn();
        closeFn = null;
      }
    }
  }

  function openWs(projectId: number): Function {
    const { close } = useWebSocket(getDetectionProjectWs(projectId), {
      autoReconnect: false,
      immediate: true,
      protocols: [userStore.getToken, userStore.getCurrentSpaceId.toString()],
      onConnected: () => {},
      onError: (ws, event) => {
        console.log('连接失败', ws, event);
      },
      onDisconnected: (ws, event) => {
        console.log('连接断开', ws, event);
      },
      onMessage: (ws, event) => {
        console.log(ws, event);
        if (event.data == 'success') {
        } else if (event.data == 'error') {
          loadingRef.value = false;
        } else {
          detectionList.push(JSON.parse(event.data));
        }
      },
    });
    return close;
  }
</script>
