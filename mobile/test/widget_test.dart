import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';

import 'package:shellsync_mobile/app.dart';
import 'package:shellsync_mobile/stores/app_state.dart';

void main() {
  testWidgets('app boots to loading screen', (WidgetTester tester) async {
    await tester.pumpWidget(
      ChangeNotifierProvider(
        create: (_) => AppState(),
        child: const ShellSyncApp(),
      ),
    );
    // AppState 尚未初始化时应显示加载指示器，而不是抛异常
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
  });
}
