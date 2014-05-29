var app = angular.module("FlowApp", ['flow'])
app.config(['flowFactoryProvider', function(flowFactoryProvider) {
  flowFactoryProvider.defaults = {
    target: '/upload',
    chunkSize: 1024 * 1024 * 10,
    permanentErrors:[404, 500, 501]
  }
}])

app.controller("FlowCtrl", ["$scope", function($scope) {
  $scope.percentDone = function(file) {
    return ((file._prevUploadedSize / file.size).toFixed(4) * 100).toString() + "%";
  };

  $scope.progress = function(file) {
    return {width: $scope.percentDone(file)};
  };
}])
